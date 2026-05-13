package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"github.com/kpixiv/kpixiv/internal/logger"
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

type RankingType string

const (
	RankingDaily   RankingType = "daily"
	RankingWeekly  RankingType = "weekly"
	RankingMonthly RankingType = "monthly"

	pixivRankingURL = "https://www.pixiv.net/ranking.php"
	pixivBaseURL   = "https://www.pixiv.net"

	desktopFirefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0"
	pixivAppUA       = "PixivIOSApp/7.6.2 (iOS 14.6; iPhone13,2)"
)

type PixivImageClient interface {
	FetchRanking(ctx context.Context, rankingType RankingType, page int, r18 bool) ([]Image, error)
	DownloadImage(ctx context.Context, image Image, destPath string) error
}

type Client struct {
	rankingClient *http.Client
	imageClient   *http.Client
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

	return &Client{
		rankingClient: rankingClient,
		imageClient:   imageClient,
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

func (c *Client) FetchRanking(ctx context.Context, rankingType RankingType, page int, r18 bool) ([]Image, error) {
	mode := string(rankingType)
	if r18 {
		mode = mode + "_r18"
	}

	log := logger.WithComponent("pixiv")
	log.Info("Fetching ranking", "page", page, "mode", mode, "r18", r18)

	targetURL := fmt.Sprintf("%s?format=json&mode=%s&content=illust&p=%d", pixivRankingURL, mode, page)
	log.Debug("Request URL", "url", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", desktopFirefoxUA)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Referer", pixivBaseURL+"/")

	resp, err := c.rankingClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ranking: %w", err)
	}
	defer resp.Body.Close()

	log.Info("Response status", "status", resp.StatusCode, "size", resp.ContentLength)

	contentType := resp.Header.Get("Content-Type")
	log.Debug("Content-Type", "type", contentType)

	if !strings.Contains(contentType, "application/json") {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		bodyStr := string(body)
		if len(body) == 1024 {
			bodyStr = bodyStr + "... [truncated]"
		}

		if strings.Contains(bodyStr, "<!DOCTYPE html>") || strings.Contains(bodyStr, "<html") {
			return nil, fmt.Errorf("received HTML instead of JSON (likely Cloudflare challenge). "+
				"Content-Type: %s, Body preview: %s", contentType, bodyStr[:min(200, len(bodyStr))])
		}

		return nil, fmt.Errorf("unexpected Content-Type: %s, body: %s", contentType, bodyStr)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Debug("Response size", "bytes", len(body))

	var rankingResp RankingResponse
	if err := json.Unmarshal(body, &rankingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

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

	log.Info("Fetched images", "count", len(images))
	return images, nil
}

func (c *Client) DownloadImage(ctx context.Context, image Image, destPath string) error {
	log := logger.WithComponent("pixiv")
	log.Info("Downloading image", "id", image.ID, "url", image.URL, "dest", destPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, image.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", pixivAppUA)
	req.Header.Set("Referer", pixivBaseURL+"/")

	resp, err := c.imageClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if _, err := os.Stat(destPath); err == nil {
		if err := os.Remove(destPath); err != nil {
			log.Warn("Failed to remove existing file", "path", destPath, "error", err)
		}
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	log.Info("Image downloaded", "path", destPath, "bytes", written)
	return nil
}