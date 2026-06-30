package pixiv

import (
	"context"
	"crypto/md5" //nolint:gosec // required to match Pixiv Android app X-Client-Hash
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
)

const (
	pixivLoginURL        = "https://app-api.pixiv.net/web/v1/login"
	pixivAuthTokenURL    = "https://oauth.secure.pixiv.net/auth/token" //nolint:gosec // Pixiv API endpoint URL, not a secret
	pixivAuthRedirectURI = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback"
	pixivAuthClientID    = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	pixivAuthSecret      = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj" //nolint:gosec // public Pixiv Android client secret
	pixivAuthUserAgent   = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	pixivClientHashSalt  = "28c1fdd170a5204386cb1313c7077b34f83e4aaf4aa829ce78c231e05b0bae2c"
)

type AuthState struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
}

type LoginFlow struct {
	URL          string
	CodeVerifier string
}

type authTokenResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresIn    int           `json:"expires_in"`
	User         authUserBrief `json:"user"`
}

type authUserBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) sessionPath() string {
	if c.stateDir == "" {
		return ""
	}

	return filepath.Join(c.stateDir, "pixiv_session.json")
}

// LoggedIn reports whether a persisted Pixiv refresh token is available.
func (c *Client) LoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.auth.RefreshToken != ""
}

// AuthUserName returns the logged-in user's display name.
func (c *Client) AuthUserName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.auth.UserName
}

// AuthUserID returns the logged-in user's Pixiv user ID.
func (c *Client) AuthUserID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.auth.UserID
}

// Logout clears the persisted Pixiv session.
func (c *Client) Logout() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth = AuthState{}

	path := c.sessionPath()
	if path == "" {
		return nil
	}

	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear pixiv session: %w", err)
	}

	return nil
}

// BeginLogin creates the Pixiv PKCE login URL and verifier.
func (c *Client) BeginLogin() (*LoginFlow, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}

	challenge := codeChallenge(verifier)
	params := url.Values{}
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("client", "pixiv-android")

	return &LoginFlow{
		URL:          pixivLoginURL + "?" + params.Encode(),
		CodeVerifier: verifier,
	}, nil
}

// FinishLogin exchanges a Pixiv callback code for a persisted session.
func (c *Client) FinishLogin(ctx context.Context, verifier, rawCode string) (*AuthState, error) {
	code, state, err := extractAuthParams(rawCode)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"client_id":      {pixivAuthClientID},
		"client_secret":  {pixivAuthSecret},
		"code":           {code},
		"code_verifier":  {verifier},
		"grant_type":     {"authorization_code"},
		"include_policy": {"true"},
		"redirect_uri":   {pixivAuthRedirectURI},
	}
	if state != "" {
		params["state"] = []string{state}
	}

	resp, err := c.exchangeToken(ctx, params)
	if err != nil {
		return nil, err
	}

	s := AuthState{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		UserID:       resp.User.ID,
		UserName:     resp.User.Name,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth = s
	if err = c.saveAuthStateLocked(); err != nil {
		return nil, err
	}

	return &s, nil
}

// BookmarkIllust bookmarks an artwork for the logged-in Pixiv account.
func (c *Client) BookmarkIllust(ctx context.Context, illustID string) error {
	accessToken, err := c.ensureAccessToken(ctx)
	if err != nil {
		return err
	}

	body := url.Values{}
	body.Set("illust_id", illustID)
	body.Set("restrict", "public")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://app-api.pixiv.net/v2/illust/bookmark/add", strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create bookmark request: %w", err)
	}

	applyPixivAppHeaders(req)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.authClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to bookmark artwork: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close on best-effort basis

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyText, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // limited read, error not actionable
		return fmt.Errorf("bookmark request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyText)))
	}

	return nil
}

func (c *Client) ensureAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.auth.RefreshToken == "" {
		c.mu.Unlock()
		return "", fmt.Errorf("login required")
	}

	if c.auth.AccessToken != "" && time.Until(c.auth.ExpiresAt) > 30*time.Second {
		token := c.auth.AccessToken
		c.mu.Unlock()
		return token, nil
	}

	refreshToken := c.auth.RefreshToken
	c.mu.Unlock()

	resp, err := c.exchangeToken(ctx, url.Values{
		"client_id":      {pixivAuthClientID},
		"client_secret":  {pixivAuthSecret},
		"grant_type":     {"refresh_token"},
		"include_policy": {"true"},
		"refresh_token":  {refreshToken},
	})
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth.AccessToken = resp.AccessToken
	if resp.RefreshToken != "" {
		c.auth.RefreshToken = resp.RefreshToken
	}
	c.auth.ExpiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	if resp.User.ID != "" {
		c.auth.UserID = resp.User.ID
	}
	if resp.User.Name != "" {
		c.auth.UserName = resp.User.Name
	}
	if err = c.saveAuthStateLocked(); err != nil {
		return "", err
	}

	return c.auth.AccessToken, nil
}

func (c *Client) exchangeToken(ctx context.Context, form url.Values) (*authTokenResponse, error) {
	bodyStr := form.Encode()
	logger.WithComponent("pixiv").Debug("token request body", "body", bodyStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pixivAuthTokenURL, strings.NewReader(bodyStr))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("User-Agent", pixivAuthUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenHTTP := &http.Client{Timeout: 30 * time.Second}
	resp, err := tokenHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to talk to pixiv auth endpoint: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close on best-effort basis

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	logger.WithComponent("pixiv").Debug("token response", "status", resp.StatusCode, "body", string(body))

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("pixiv auth failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token authTokenResponse
	if err = json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if token.AccessToken == "" || token.RefreshToken == "" {
		return nil, fmt.Errorf("pixiv auth response did not include usable tokens")
	}

	return &token, nil
}

func (c *Client) loadAuthState() error {
	path := c.sessionPath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read pixiv session: %w", err)
	}

	var state AuthState
	if err = json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to decode pixiv session: %w", err)
	}

	c.mu.Lock()
	c.auth = state
	c.mu.Unlock()
	return nil
}

func (c *Client) saveAuthStateLocked() error {
	path := c.sessionPath()
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("failed to create pixiv session directory: %w", err)
	}

	data, err := json.MarshalIndent(c.auth, "", "  ") //nolint:gosec // access_token field name matches secret pattern but is intentional
	if err != nil {
		return fmt.Errorf("failed to encode pixiv session: %w", err)
	}

	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to persist pixiv session: %w", err)
	}

	return nil
}

func extractAuthParams(input string) (code, state string, err error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", fmt.Errorf("pixiv login code is empty")
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("invalid pixiv callback URL: %w", err)
		}

		code = strings.TrimSpace(parsed.Query().Get("code"))
		if code == "" {
			return "", "", fmt.Errorf("pixiv callback URL did not contain a code parameter")
		}

		state = strings.TrimSpace(parsed.Query().Get("state"))
		return code, state, nil
	}

	return trimmed, "", nil
}

func generateCodeVerifier() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate pixiv login verifier: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func applyPixivAppHeaders(req *http.Request) {
	clientTime := time.Now().UTC().Format(time.RFC3339)
	hash := md5.Sum([]byte(clientTime + pixivClientHashSalt)) //nolint:gosec // required to match Pixiv Android app X-Client-Hash

	req.Header.Set("User-Agent", pixivAuthUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Accept-Language", "en_US")
	req.Header.Set("App-Accept-Language", "en")
	req.Header.Set("App-OS", "android")
	req.Header.Set("App-OS-Version", "11")
	req.Header.Set("App-Version", "6.96.0")
	req.Header.Set("X-Client-Time", clientTime)
	req.Header.Set("X-Client-Hash", hex.EncodeToString(hash[:]))
}
