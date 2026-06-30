package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv/resolver"
)

type Image struct {
	Timestamp time.Time
	ID        string
	URL       string
	Title     string
	Artist    string
	ArtistID  string
	Width     int
	Height    int
	Rank      int
}

const (
	pixivRankingURL = "https://www.pixiv.net/ranking.php"
	pixivBaseURL    = "https://www.pixiv.net"

	desktopFirefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0"
	pixivAppUA       = "PixivIOSApp/7.6.2 (iOS 14.6; iPhone13,2)"
)

type ImageClient interface {
	FetchRanking(ctx context.Context, rankingMode string, page int, r18 bool) ([]Image, int, error)
	DownloadImage(ctx context.Context, image *Image, destPath string) error
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type OriginalURLResolver interface {
	ResolveOriginalURL(ctx context.Context, thumbnailURL string) (string, error)
}

type Client struct {
	rankingClient HTTPClient
	imageClient   HTTPClient
	authClient    HTTPClient
	resolver      OriginalURLResolver
	stateDir      string
	auth          AuthState
	mu            sync.Mutex
}

// NewClient constructs a Pixiv client for ranking and image downloads.
func NewClient(stateDir string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	rankingTransport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		MaxConnsPerHost:     5,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	rankingClient := &http.Client{
		Jar:       jar,
		Transport: rankingTransport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}

			if req.URL.Path == "/login.php" {
				return fmt.Errorf("redirected to login page")
			}

			return nil
		},
	}

	imageTransport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     60 * time.Second,
		DisableCompression:  false,
		MaxConnsPerHost:     5,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	imageClient := &http.Client{
		Jar:       jar,
		Transport: imageTransport,
		Timeout:   60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	r, rErr := resolver.NewResolver()
	if rErr != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", rErr)
	}

	client := &Client{
		rankingClient: rankingClient,
		imageClient:   imageClient,
		authClient:    imageClient,
		resolver:      r,
		stateDir:      stateDir,
	}

	if err = client.loadAuthState(); err != nil {
		return nil, err
	}

	return client, nil
}

type RankingResponse struct {
	Contents []RankingContent `json:"contents"`
	Next     any              `json:"next"`
}

type RankingContent struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	UserName string `json:"user_name"`
	IllustID int64  `json:"illust_id"`
	UserID   int64  `json:"user_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Rank     int    `json:"rank"`
	IsMasked bool   `json:"is_masked"`
}

// FetchRanking fetches ranking entries and returns parsed image candidates.
func (c *Client) FetchRanking(ctx context.Context, rankingMode string, page int, r18 bool) ([]Image, int, error) {
	mode := rankingMode
	if r18 {
		mode += "_r18"
	}

	log := logger.WithComponent("pixiv")
	log.Info("Fetching ranking", "page", page, "mode", mode, "r18", r18)

	targetURL := fmt.Sprintf("%s?format=json&mode=%s&content=illust&p=%d", pixivRankingURL, mode, page)
	log.Debug("Request URL", "url", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 1, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", desktopFirefoxUA)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Referer", pixivBaseURL+"/")

	resp, err := c.rankingClient.Do(req)
	if err != nil {
		return nil, 1, fmt.Errorf("failed to fetch ranking: %w", err)
	}

	//nolint:errcheck
	defer func() { _ = resp.Body.Close() }()

	log.Debug("Response status", "status", resp.StatusCode, "size", resp.ContentLength)

	contentType := resp.Header.Get("Content-Type")
	log.Debug("Content-Type", "type", contentType)

	if !strings.Contains(contentType, "application/json") {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return nil, 1, fmt.Errorf("failed to read response body: %w", readErr)
		}

		bodyStr := string(body)
		if len(body) == 1024 {
			bodyStr += "... [truncated]"
		}

		if strings.Contains(bodyStr, "<!DOCTYPE html>") || strings.Contains(bodyStr, "<html") {
			return nil, 1, fmt.Errorf("received HTML instead of JSON (likely Cloudflare challenge). "+
				"Content-Type: %s, Body preview: %s", contentType, bodyStr[:min(200, len(bodyStr))])
		}

		return nil, 1, fmt.Errorf("unexpected Content-Type: %s, body: %s", contentType, bodyStr)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 1, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Debug("Response size", "bytes", len(body))

	var rankingResp RankingResponse
	if err = json.Unmarshal(body, &rankingResp); err != nil {
		return nil, 1, fmt.Errorf("failed to decode response: %w", err)
	}

	nextPage := parseNextPage(rankingResp.Next)

	images := make([]Image, 0, len(rankingResp.Contents))
	for _, rc := range rankingResp.Contents {
		if rc.IsMasked && !r18 {
			continue
		}

		img := Image{
			ID:        fmt.Sprintf("%d", rc.IllustID),
			URL:       rc.URL,
			Width:     rc.Width,
			Height:    rc.Height,
			Title:     rc.Title,
			Artist:    rc.UserName,
			ArtistID:  fmt.Sprintf("%d", rc.UserID),
			Rank:      rc.Rank,
			Timestamp: time.Now(),
		}
		images = append(images, img)
	}

	log.Debug("Fetched images", "count", len(images))
	log.Debug("Ranking pagination", "currentPage", page, "nextPage", nextPage)
	return images, nextPage, nil
}

type bookmarkIllustsResponse struct {
	Illusts []bookmarkIllust `json:"illusts"`
	NextURL string           `json:"next_url"`
}

type bookmarkIllust struct {
	ID        int64             `json:"id"`
	Title     string            `json:"title"`
	User      bookmarkUser      `json:"user"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	ImageURLs bookmarkImageURLs `json:"image_urls"`
	Visible   bool              `json:"visible"`
}

type bookmarkUser struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type bookmarkImageURLs struct {
	Large  string `json:"large"`
	Medium string `json:"medium"`
}

// FetchBookmarks fetches bookmarked images for a user and returns parsed images.
// Pass an empty string for "nextURL" to fetch the first page.
// Returns the nextURL for the subsequent page (empty string when done).
func (c *Client) FetchBookmarks(ctx context.Context, userID string, nextURL string) ([]Image, string, error) {
	accessToken, err := c.ensureAccessToken(ctx)
	if err != nil {
		return nil, "", err
	}

	log := logger.WithComponent("pixiv")

	targetURL := nextURL
	if targetURL == "" {
		targetURL = fmt.Sprintf("https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=%s&restrict=public", url.QueryEscape(userID))
	}
	log.Debug("Fetching bookmarks", "url", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	applyPixivAppHeaders(req)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.authClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch bookmarks: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck
		return nil, "", fmt.Errorf("bookmarks request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read bookmarks response: %w", err)
	}

	var data bookmarkIllustsResponse
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, "", fmt.Errorf("failed to decode bookmarks response: %w", err)
	}

	images := make([]Image, 0, len(data.Illusts))
	for _, bi := range data.Illusts {
		if !bi.Visible {
			log.Debug("Skipping invisible/deleted images", "id", bi.ID)
			continue
		}

		imgURL := bi.ImageURLs.Large
		if imgURL == "" {
			imgURL = bi.ImageURLs.Medium
		}

		img := Image{
			ID:        fmt.Sprintf("%d", bi.ID),
			URL:       imgURL,
			Width:     bi.Width,
			Height:    bi.Height,
			Title:     bi.Title,
			Artist:    bi.User.Name,
			ArtistID:  fmt.Sprintf("%d", bi.User.ID),
			Timestamp: time.Now(),
		}
		images = append(images, img)
	}

	next := data.NextURL
	log.Debug("Fetched bookmarks", "count", len(images), "skipped", len(data.Illusts)-len(images), "hasNext", next != "")
	return images, next, nil
}

func parseNextPage(next any) int {
	switch v := next.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return 1
}

// DownloadImage downloads an image to destination path atomically.
func (c *Client) DownloadImage(ctx context.Context, image *Image, destPath string) error {
	log := logger.WithComponent("pixiv")
	downloadURL := image.URL

	if strings.Contains(downloadURL, "/img-master/") && c.resolver != nil {
		originalURL, err := c.resolver.ResolveOriginalURL(ctx, image.URL)
		if err != nil {
			return fmt.Errorf("failed to resolve original URL for %s: %w", image.ID, err)
		}

		downloadURL = originalURL
		if u, err := url.Parse(downloadURL); err == nil {
			if ext := path.Ext(u.Path); ext != "" {
				basePath := strings.TrimSuffix(destPath, filepath.Ext(destPath))
				destPath = basePath + ext
			}
		}
	}

	log.Debug("Downloading image", "id", image.ID, "url", downloadURL, "dest", destPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", pixivAppUA)
	req.Header.Set("Referer", pixivBaseURL+"/")

	resp, err := c.imageClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}

	//nolint:errcheck
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	destDir := filepath.Dir(destPath)
	if err = os.MkdirAll(destDir, 0750); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(destDir, ".download-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	tmpPath := tmpFile.Name()
	defer func() {
		//nolint:errcheck
		_ = tmpFile.Close()
		//nolint:errcheck
		_ = os.Remove(tmpPath)
	}()

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	if err = tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to flush temporary file: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	if err = os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to move temporary file: %w", err)
	}

	log.Debug("Image downloaded", "path", destPath, "bytes", written)
	return nil
}
