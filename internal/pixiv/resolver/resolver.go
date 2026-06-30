package resolver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
)

const (
	pixivReferer     = "https://www.pixiv.net/"
	desktopFirefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0"
)

var supportedExtensions = []string{".png", ".jpg"}

type Resolver struct {
	client *http.Client
}

// NewResolver creates a URL resolver for Pixiv original image links.
func NewResolver() (*Resolver, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		MaxConnsPerHost:     5,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return &Resolver{client: client}, nil
}

// ResolveOriginalURL resolves a thumbnail URL to a downloadable original URL.
func (r *Resolver) ResolveOriginalURL(ctx context.Context, thumbnailURL string) (string, error) {
	log := logger.WithComponent("resolver")

	baseURL, err := transformThumbnailToOriginalBase(thumbnailURL)
	if err != nil {
		return "", fmt.Errorf("failed to transform URL: %w", err)
	}

	log.Debug("Resolved base URL", "base", baseURL)

	for _, ext := range supportedExtensions {
		candidateURL := baseURL + ext
		log.Debug("Probing extension", "url", candidateURL, "ext", ext)

		if valid, err := r.validateURL(ctx, candidateURL); err != nil {
			log.Warn("Validation error", "url", candidateURL, "error", err)
			continue
		} else if valid {
			log.Debug("Found valid original URL", "url", candidateURL)
			return candidateURL, nil
		}
	}

	return "", fmt.Errorf("no valid original URL found for thumbnail: %s", thumbnailURL)
}

func transformThumbnailToOriginalBase(thumbnailURL string) (string, error) {
	parsed, err := url.Parse(thumbnailURL)
	if err != nil {
		return "", fmt.Errorf("invalid thumbnail URL: %w", err)
	}

	path := parsed.Path

	if !strings.Contains(path, "/img-master/") {
		return "", fmt.Errorf("thumbnail URL does not contain /img-master/: %s", thumbnailURL)
	}

	re := regexp.MustCompile(`/c/[^/]+`)
	transformedPath := re.ReplaceAllString(path, "")

	transformedPath = strings.Replace(transformedPath, "/img-master/", "/img-original/", 1)

	transformedPath = strings.Replace(transformedPath, "_master1200", "", 1)

	if len(transformedPath) < 4 {
		return "", fmt.Errorf("unexpected thumbnail path: %s", thumbnailURL)
	}

	if strings.HasSuffix(transformedPath, ".jpg") || strings.HasSuffix(transformedPath, ".png") {
		transformedPath = transformedPath[:len(transformedPath)-4]
	}

	parsed.Path = transformedPath

	return parsed.String(), nil
}

func (r *Resolver) validateURL(ctx context.Context, imageURL string) (bool, error) {
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, imageURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	headReq.Header.Set("User-Agent", desktopFirefoxUA)
	headReq.Header.Set("Referer", pixivReferer)

	resp, err := r.client.Do(headReq)
	if err == nil {
		//nolint:errcheck
		defer func() { _ = resp.Body.Close() }()
		if IsImageResponse(resp) && resp.ContentLength != 0 {
			return true, nil
		}
		if resp.StatusCode != http.StatusMethodNotAllowed &&
			resp.StatusCode != http.StatusNotImplemented {
			//nolint:errcheck
			_ = resp.Body.Close()
			return false, nil
		}
	}

	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create fallback GET request: %w", err)
	}
	getReq.Header.Set("User-Agent", desktopFirefoxUA)
	getReq.Header.Set("Referer", pixivReferer)

	resp, err = r.client.Do(getReq)
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}
	//nolint:errcheck
	defer func() { _ = resp.Body.Close() }()

	if !IsImageResponse(resp) {
		return false, nil
	}

	return HasContent(resp), nil
}

// TruncateString truncates a string and appends ellipsis when needed.
func TruncateString(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
