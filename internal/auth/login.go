package auth

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
)

const authComponent = "auth"

type SchemeHandler struct {
	desktopDir  string
	desktopFile string
	scriptPath  string
}

// Provider supplies the Pixiv OAuth steps that auth.Login orchestrates.
type Provider interface {
	// BeginLogin starts the login flow and returns the PKCE auth URL and verifier.
	BeginLogin() (authURL string, verifier string, err error)
	// FinishLogin exchanges the authorization code for tokens and returns the user name.
	FinishLogin(ctx context.Context, verifier, code string) (userName string, err error)
}

// LoginConfig controls the OAuth login flow.
type LoginConfig struct {
	// Timeout is the maximum time to wait for authentication.
	// Defaults to 5 minutes if zero.
	Timeout time.Duration
}

// Login performs the complete OAuth login flow:
//  1. Generates PKCE verifier and challenge
//  2. Starts a localhost callback server
//  3. Registers a temporary pixiv:// scheme handler
//  4. Opens the browser to the Pixiv authorization URL
//  5. Waits for the callback with the authorization code
//  6. Exchanges the code for tokens
//  7. Cleans up all temporary resources
func Login(ctx context.Context, cfg LoginConfig, provider Provider) (userName string, err error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	loginCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	authURL, verifier, err := provider.BeginLogin()
	if err != nil {
		return "", fmt.Errorf("failed to start login: %w", err)
	}

	cs := newCallbackServer()
	port, err := cs.Start(loginCtx)
	if err != nil {
		return "", fmt.Errorf("failed to start callback server: %w", err)
	}

	handler, err := registerSchemeHandler(port)
	if err != nil {
		cs.shutdown() //nolint:contextcheck // fresh context created inside shutdown
		return "", fmt.Errorf("failed to register scheme handler: %w", err)
	}

	defer handler.cleanup()

	if browserErr := openBrowser(authURL); browserErr != nil {
		cs.shutdown() //nolint:contextcheck // fresh context created inside shutdown
		return "", fmt.Errorf("failed to open browser: %w", browserErr)
	}

	rawURL, err := cs.WaitForResult(loginCtx)
	cs.shutdown() //nolint:contextcheck // fresh context created inside shutdown

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("authentication timed out after %v", timeout)
		}
		if errors.Is(err, context.Canceled) {
			return "", fmt.Errorf("authentication cancelled")
		}
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	code, err := extractCodeFromPixivURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to extract authorization code: %w", err)
	}

	logger.WithComponent(authComponent).Info("token exchange started")

	userName, err = provider.FinishLogin(ctx, verifier, code)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}

	logger.WithComponent(authComponent).Info("token exchange complete")

	return userName, nil
}

func openBrowser(targetURL string) error {
	logger.WithComponent(authComponent).Debug("opening browser", "url", targetURL)
	cmd := exec.Command("xdg-open", targetURL) //nolint:gosec // URL is generated internally by PKCE flow
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}
	return nil
}

func extractCodeFromPixivURL(rawURL string) (string, error) {
	if !strings.Contains(rawURL, "://") {
		return "", fmt.Errorf("invalid callback URL: missing scheme")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid callback URL: %w", err)
	}

	code := strings.TrimSpace(parsed.Query().Get("code"))
	if code == "" {
		return "", fmt.Errorf("callback URL did not contain a code parameter")
	}

	return code, nil
}

func registerSchemeHandler(port int) (*SchemeHandler, error) {
	tmpDir := filepath.Join(os.TempDir(), "kpixiv-auth")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("handler-%d.sh", port))
	script := fmt.Sprintf(`#!/bin/sh
curl -sf "http://127.0.0.1:%d/callback?url=%s" >/dev/null 2>&1
`, port, "%s")

	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil { //nolint:gosec // script must be executable for scheme handler
		return nil, fmt.Errorf("failed to write handler script: %w", err)
	}

	desktopFilePath := filepath.Join(tmpDir, fmt.Sprintf("kpixiv-auth-%d.desktop", port))
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=kPixiv Auth Handler
Exec=%s %%u
NoDisplay=true
MimeType=x-scheme-handler/pixiv
`, html.EscapeString(scriptPath))

	if err := os.WriteFile(desktopFilePath, []byte(desktopContent), 0600); err != nil {
		os.Remove(scriptPath) //nolint:errcheck,gosec // best-effort cleanup
		return nil, fmt.Errorf("failed to write desktop file: %w", err)
	}

	cmd := exec.Command("xdg-mime", "default", filepath.Base(desktopFilePath), "x-scheme-handler/pixiv") //nolint:gosec // paths are generated internally
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(scriptPath)      //nolint:errcheck,gosec // best-effort cleanup
		os.Remove(desktopFilePath) //nolint:errcheck,gosec // best-effort cleanup
		return nil, fmt.Errorf("failed to register pixiv:// scheme handler: %w\n%s", err, string(out))
	}

	logger.WithComponent(authComponent).Debug("pixiv:// scheme handler registered", "port", port)

	return &SchemeHandler{
		desktopDir:  tmpDir,
		desktopFile: desktopFilePath,
		scriptPath:  scriptPath,
	}, nil
}

func (h *SchemeHandler) cleanup() {
	if h.desktopFile != "" {
		if err := exec.Command("xdg-mime", "default", "", "x-scheme-handler/pixiv").Run(); err != nil {
			logger.WithComponent(authComponent).Warn("failed to unregister scheme handler", "error", err)
		}
		os.Remove(h.desktopFile) //nolint:errcheck,gosec // best-effort cleanup
	}
	if h.scriptPath != "" {
		os.Remove(h.scriptPath) //nolint:errcheck,gosec // best-effort cleanup
	}
	if h.desktopDir != "" {
		os.Remove(h.desktopDir) //nolint:errcheck,gosec // best-effort cleanup
	}
	logger.WithComponent(authComponent).Info("scheme handler cleaned up")
}
