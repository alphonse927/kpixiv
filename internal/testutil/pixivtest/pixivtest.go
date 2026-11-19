// Package pixivtest provides a fake of the Pixiv ranking/download client for
// tests of the packages that consume it.
package pixivtest

import (
	"context"
	"os"

	"github.com/alphonse927/kpixiv/internal/pixiv"
)

// Client is a configurable fake with FetchRanking and DownloadImage methods.
// The zero value serves an empty ranking and downloads nothing.
type Client struct {
	// Images and NextPage are what FetchRanking returns.
	Images   []pixiv.Image
	NextPage int

	// FetchErr, when set, makes FetchRanking fail.
	FetchErr error

	// DownloadErr is what DownloadImage returns when DownloadFunc is nil.
	DownloadErr error

	// DownloadFunc, when set, replaces the default DownloadImage behavior.
	DownloadFunc func(ctx context.Context, img *pixiv.Image, destPath string) error

	// OnFetch, when set, runs at the start of every FetchRanking call.
	OnFetch func()
}

// NewWriting returns a Client that serves images (with a following page) and
// writes a small file at the destination path on every download.
func NewWriting(images []pixiv.Image) *Client {
	return &Client{
		Images:   images,
		NextPage: 2,
		DownloadFunc: func(_ context.Context, _ *pixiv.Image, destPath string) error {
			return os.WriteFile(destPath, []byte("img"), 0600)
		},
	}
}

// FetchRanking implements the ranking half of the Pixiv client.
func (c *Client) FetchRanking(context.Context, string, int, bool) ([]pixiv.Image, int, error) {
	if c.OnFetch != nil {
		c.OnFetch()
	}

	if c.FetchErr != nil {
		return nil, 1, c.FetchErr
	}

	return c.Images, c.NextPage, nil
}

// DownloadImage implements the download half of the Pixiv client.
func (c *Client) DownloadImage(ctx context.Context, img *pixiv.Image, destPath string) error {
	if c.DownloadFunc != nil {
		return c.DownloadFunc(ctx, img, destPath)
	}

	return c.DownloadErr
}
