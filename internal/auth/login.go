package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
)

const authComponent = "auth"

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
	// OnAuthURL is called with the Pixiv authorization URL before the
	// browser is opened, so the caller can display it in the UI.
	OnAuthURL func(url string)
}

// Login performs the complete OAuth login flow:
//  1. Generates PKCE verifier and challenge
//  2. Starts a Unix socket receiver for the pixiv:// callback
//  3. Opens the browser to the Pixiv authorization URL
//  4. Waits for the callback with the authorization code
//  5. Exchanges the code for tokens
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

	if cfg.OnAuthURL != nil {
		cfg.OnAuthURL(authURL)
	}

	sr, err := newSocketReceiver()
	if err != nil {
		return "", fmt.Errorf("failed to start callback receiver: %w", err)
	}
	defer sr.Close()

	if browserErr := openBrowser(authURL); browserErr != nil {
		return "", fmt.Errorf("failed to open browser: %w", browserErr)
	}

	rawURL, err := sr.Wait(loginCtx)
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
	logger.WithComponent(authComponent).Info("Opening browser...", "url", targetURL)
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
