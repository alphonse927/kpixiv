package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv/resolver"
)

type Image struct {
	ID        string
	URL       string
	Width     int
	Height    int
	Title     string
	Artist    string
	ArtistID  string
	Timestamp time.Time
}

const (
	pixivRankingURL = "https://www.pixiv.net/ranking.php"
	pixivBaseURL    = "https://www.pixiv.net"

	desktopFirefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0"
	pixivAppUA       = "PixivIOSApp/7.6.2 (iOS 14.6; iPhone13,2)"
)

type PixivImageClient interface {
	FetchRanking(ctx context.Context, rankingMode string, page int, r18 bool) ([]Image, int, error)
	DownloadImage(ctx context.Context, image *Image, destPath string) error
}

type Client struct {
	rankingClient *http.Client
	imageClient   *http.Client
	resolver      *resolver.Resolver
}

func NewClient() (*Client, error) {
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

	r, err := resolver.NewResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	return &Client{
		rankingClient: rankingClient,
		imageClient:   imageClient,
		resolver:      r,
	}, nil
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
	IsMasked bool   `json:"is_masked"`
}

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
			Timestamp: time.Now(),
		}
		images = append(images, img)
	}

	log.Debug("Fetched images", "count", len(images))
	log.Debug("Ranking pagination", "currentPage", page, "nextPage", nextPage)
	return images, nextPage, nil
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

func (c *Client) DownloadImage(ctx context.Context, image *Image, destPath string) error {
	log := logger.WithComponent("pixiv")
	downloadURL := image.URL
	if strings.Contains(downloadURL, "/img-master/") {
		originalURL, err := c.resolver.ResolveOriginalURL(ctx, image.URL)
		if err != nil {
			return fmt.Errorf("failed to resolve original URL for %s: %w", image.ID, err)
		}
		downloadURL = originalURL
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

	tmpFile, err := os.CreateTemp(destDir, ".kpixiv-*.tmp")
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
		return fmt.Errorf("failed to move temporary file into place: %w", err)
	}

	log.Debug("Image downloaded", "path", destPath, "bytes", written)
	return nil
}
