package cache

import (
	"context"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
)

type Cache struct {
	mu         sync.RWMutex
	images     []pixiv.Image
	storage    *storage.Storage
	lastFetch  time.Time
	fetchedIDs map[string]bool
}

func NewCache(st *storage.Storage) *Cache {
	return &Cache{
		storage:    st,
		images:     []pixiv.Image{},
		fetchedIDs: make(map[string]bool),
	}
}

func (c *Cache) Add(images []pixiv.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, img := range images {
		if !c.fetchedIDs[img.ID] {
			c.images = append(c.images, img)
			c.fetchedIDs[img.ID] = true
		}
	}
	c.lastFetch = time.Now()
}

func (c *Cache) GetAll() []pixiv.Image {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]pixiv.Image, len(c.images))
	copy(result, c.images)
	return result
}

func (c *Cache) GetFiltered(minWidth, minHeight int, landscapeOnly bool) []pixiv.Image {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []pixiv.Image
	for _, img := range c.images {
		if img.Width < minWidth || img.Height < minHeight {
			continue
		}
		if landscapeOnly && img.Height > img.Width {
			continue
		}
		result = append(result, img)
	}

	return result
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.images)
}

func (c *Cache) NeedsFetch() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.images) < 10 || time.Since(c.lastFetch) > 30*time.Minute
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.images = []pixiv.Image{}
	c.fetchedIDs = make(map[string]bool)
}

func (c *Cache) Fetch(ctx context.Context, client pixiv.PixivImageClient, rankingType pixiv.RankingType, page int, r18 bool) (int, error) {
	images, nextPage, err := client.FetchRanking(ctx, rankingType, page, r18)
	if err != nil {
		return 1, err
	}

	c.Add(images)
	return nextPage, nil
}
