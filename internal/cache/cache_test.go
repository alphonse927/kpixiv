package cache

import (
	"context"
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/pixiv"
)

type mockClient struct {
	images   []pixiv.Image
	nextPage int
	fetchErr error
}

func (m *mockClient) FetchRanking(context.Context, string, int, bool) ([]pixiv.Image, int, error) {
	if m.fetchErr != nil {
		return nil, 1, m.fetchErr
	}
	return m.images, m.nextPage, nil
}

func (m *mockClient) DownloadImage(context.Context, *pixiv.Image, string) error {
	return nil
}

type mockError struct{}

func (m mockError) Error() string { return "mock error" }

func TestNewCache(t *testing.T) {
	c := NewCache(nil)
	if c == nil {
		t.Fatal("NewCache(nil) returned nil")
	}
}

func TestAdd(t *testing.T) {
	c := NewCache(nil)

	img := []pixiv.Image{{ID: "12345", Width: 1920, Height: 1080}}
	c.Add(img)

	if len(c.GetAll()) != 1 {
		t.Errorf("Add() count: got %d, want 1", len(c.GetAll()))
	}
}

func TestAddSkipsDuplicates(t *testing.T) {
	c := NewCache(nil)

	img := []pixiv.Image{{ID: "12345", Width: 1920, Height: 1080}}
	c.Add(img)
	c.Add(img)

	if len(c.GetAll()) != 1 {
		t.Errorf("Add() duplicate: got %d, want 1", len(c.GetAll()))
	}
}

func TestGetAll(t *testing.T) {
	c := NewCache(nil)

	c.Add([]pixiv.Image{{ID: "1", Width: 1920, Height: 1080}})
	c.Add([]pixiv.Image{{ID: "2", Width: 2560, Height: 1440}})

	all := c.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll() count: got %d, want 2", len(all))
	}
}

func TestGetFiltered(t *testing.T) {
	c := NewCache(nil)

	c.Add([]pixiv.Image{{ID: "1", Width: 1920, Height: 1080}})
	c.Add([]pixiv.Image{{ID: "2", Width: 3840, Height: 2160}})

	filtered := c.GetFiltered(3840, 2160, false)
	if len(filtered) != 1 {
		t.Errorf("GetFiltered(3840, 2160, false): got %d, want 1", len(filtered))
	}
}

func TestGetFilteredWithoutLandscapeOnly(t *testing.T) {
	c := NewCache(nil)

	c.Add([]pixiv.Image{{ID: "1", Width: 1080, Height: 1920}})
	c.Add([]pixiv.Image{{ID: "2", Width: 1920, Height: 1080}})

	filtered := c.GetFiltered(1920, 1080, true)
	if len(filtered) != 1 {
		t.Errorf("GetFiltered(1920, 1080, true) landscape only: got %d, want 1", len(filtered))
	}
}

func TestNeedsFetch(t *testing.T) {
	c := NewCache(nil)

	if !c.NeedsFetch() {
		t.Error("NeedsFetch() on empty cache: got false, want true")
	}
}

func TestClear(t *testing.T) {
	c := NewCache(nil)
	c.Add([]pixiv.Image{{ID: "1", Width: 1920, Height: 1080}})

	c.Clear()
	if len(c.GetAll()) != 0 {
		t.Errorf("Clear(): got %d, want 0", len(c.GetAll()))
	}
}

func TestFetch(t *testing.T) {
	c := NewCache(nil)

	client := &mockClient{
		images: []pixiv.Image{
			{ID: "1", Width: 1920, Height: 1080},
			{ID: "2", Width: 2560, Height: 1440},
		},
		nextPage: 2,
	}

	ctx := context.Background()
	_, err := c.Fetch(ctx, client, config.RankingDailyMode.String(), 1, false)
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(c.GetAll()) != 2 {
		t.Errorf("Fetch() count: got %d, want 2", len(c.GetAll()))
	}
}

func TestFetchError(t *testing.T) {
	c := NewCache(nil)

	client := &mockClient{
		fetchErr: mockError{},
	}

	ctx := context.Background()
	_, err := c.Fetch(ctx, client, config.RankingDailyMode.String(), 1, false)
	if err == nil {
		t.Error("Fetch() with mock error: got nil, want error")
	}
}

func TestFetchIdempotent(t *testing.T) {
	c := NewCache(nil)

	client := &mockClient{
		images: []pixiv.Image{
			{ID: "11111", Width: 1920, Height: 1080},
		},
		nextPage: 2,
	}

	ctx := context.Background()
	_, err := c.Fetch(ctx, client, config.RankingDailyMode.String(), 1, false)
	if err != nil {
		t.Fatalf("Fetch() first call returned error: %v", err)
	}
	_, err = c.Fetch(ctx, client, config.RankingDailyMode.String(), 1, false)
	if err != nil {
		t.Fatalf("Fetch() second call returned error: %v", err)
	}

	if len(c.GetAll()) != 1 {
		t.Errorf("Fetch() idempotent: got %d, want 1", len(c.GetAll()))
	}
}
