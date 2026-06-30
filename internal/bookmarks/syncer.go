package bookmarks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
)

const pageDelay = 2 * time.Second

type SyncResult struct {
	Total      int
	Downloaded int
	Deleted    int
	Skipped    int
	Failed     int
}

type Syncer struct {
	cfg     *config.Config
	storage *storage.Storage
	client  *pixiv.Client
}

func NewSyncer(cfg *config.Config, st *storage.Storage, client *pixiv.Client) *Syncer {
	return &Syncer{
		cfg:     cfg,
		storage: st,
		client:  client,
	}
}

func (s *Syncer) Sync(ctx context.Context) (*SyncResult, error) {
	log := logger.WithComponent("bookmarks")

	if s.client == nil || !s.client.LoggedIn() {
		return nil, fmt.Errorf("pixiv login required")
	}

	lastPageURL, complete, err := s.storage.GetBookmarkPagination()
	if err != nil {
		return nil, fmt.Errorf("failed to load pagination state: %w", err)
	}

	remoteIDs := make(map[string]struct{})
	var result *SyncResult

	if complete {
		result, err = s.syncIncremental(ctx, remoteIDs)
	} else {
		result, err = s.syncFull(ctx, remoteIDs, lastPageURL)
	}
	if err != nil {
		return nil, err
	}

	if len(remoteIDs) > 0 {
		ids := make([]string, 0, len(remoteIDs))
		for id := range remoteIDs {
			ids = append(ids, id)
		}
		if err := s.storage.AddBookmarks(ids); err != nil {
			log.Warn("Failed to persist synced bookmarks", "error", err)
		}
	}

	log.Info("Bookmark sync complete", "total", result.Total, "downloaded", result.Downloaded, "deleted", result.Deleted, "skipped", result.Skipped, "failed", result.Failed)
	return result, nil
}

func (s *Syncer) syncIncremental(ctx context.Context, remoteIDs map[string]struct{}) (*SyncResult, error) {
	log := logger.WithComponent("bookmarks")
	log.Debug("Incremental bookmark sync (first page only)")

	images, _, err := s.client.FetchBookmarks(ctx, s.client.AuthUserID(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bookmarks: %w", err)
	}

	for _, img := range images {
		remoteIDs[img.ID] = struct{}{}
	}

	favoritesDir := s.storage.FavoritesDir()
	pending, known, prevMeta := s.prepareDownloads(images)
	downloaded := s.downloadAndSave(ctx, pending, favoritesDir, prevMeta)

	return &SyncResult{
		Total:      len(images),
		Skipped:    known,
		Downloaded: downloaded,
		Failed:     len(pending) - downloaded,
	}, nil
}

func (s *Syncer) syncFull(ctx context.Context, remoteIDs map[string]struct{}, lastPageURL string) (*SyncResult, error) {
	log := logger.WithComponent("bookmarks")
	log.Debug("Full bookmark sync", "resume", lastPageURL != "")

	favoritesDir := s.storage.FavoritesDir()
	result := &SyncResult{}
	currentURL := lastPageURL

	for {
		images, next, err := s.client.FetchBookmarks(ctx, s.client.AuthUserID(), currentURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch bookmarks: %w", err)
		}

		for _, img := range images {
			remoteIDs[img.ID] = struct{}{}
		}

		pending, known, prevMeta := s.prepareDownloads(images)
		result.Total += len(images)
		result.Skipped += known

		downloaded := s.downloadAndSave(ctx, pending, favoritesDir, prevMeta)
		result.Downloaded += downloaded
		result.Failed += len(pending) - downloaded

		if next == "" {
			log.Debug("Bookmark full sync complete, marking pagination done")
			if err := s.storage.SetBookmarkPagination("", true); err != nil {
				log.Warn("Failed to save pagination state", "error", err)
			}
			break
		}

		log.Debug("Saving bookmark pagination cursor", "nextURL", next)
		if err := s.storage.SetBookmarkPagination(next, false); err != nil {
			log.Warn("Failed to save pagination state", "error", err)
		}
		currentURL = next

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pageDelay):
		}
	}

	if s.cfg.Bookmarks.AutoCleanup {
		deleted, err := s.cleanupRemoved(ctx, remoteIDs)
		if err != nil {
			log.Warn("Failed to cleanup removed bookmarks", "error", err)
		}
		result.Deleted = deleted
	}

	return result, nil
}

func (s *Syncer) prepareDownloads(images []pixiv.Image) (pending []pixiv.Image, known int, existingMeta map[string]*storage.ImageMeta) {
	metadata, err := s.storage.LoadMetadata()
	if err != nil {
		return images, 0, metadata
	}

	favoritesDir := s.storage.FavoritesDir()

	var pend []pixiv.Image
	knownCount := 0
	for _, img := range images {
		destPath := filepath.Join(favoritesDir, img.ID+".jpg")
		altPath := filepath.Join(favoritesDir, img.ID+".png")

		if meta, ok := metadata[img.ID]; ok && meta.Source == "favorites" {
			if _, err := os.Stat(meta.Path); err == nil {
				knownCount++
				continue
			}
		}

		if _, err := os.Stat(destPath); err == nil {
			metadata[img.ID] = &storage.ImageMeta{
				ID:           img.ID,
				Path:         destPath,
				Width:        img.Width,
				Height:       img.Height,
				Title:        img.Title,
				Artist:       img.Artist,
				ArtistID:     img.ArtistID,
				Source:       "favorites",
				DownloadedAt: time.Now(),
			}
			knownCount++
			continue
		}

		if _, err := os.Stat(altPath); err == nil {
			metadata[img.ID] = &storage.ImageMeta{
				ID:           img.ID,
				Path:         altPath,
				Width:        img.Width,
				Height:       img.Height,
				Title:        img.Title,
				Artist:       img.Artist,
				ArtistID:     img.ArtistID,
				Source:       "favorites",
				DownloadedAt: time.Now(),
			}
			knownCount++
			continue
		}

		pend = append(pend, img)
	}

	return pend, knownCount, metadata
}

func (s *Syncer) downloadAndSave(ctx context.Context, pending []pixiv.Image, favoritesDir string, metadata map[string]*storage.ImageMeta) int {
	log := logger.WithComponent("bookmarks")

	downloaded := 0
	for _, img := range pending {
		destPath := filepath.Join(favoritesDir, img.ID+".jpg")
		if err := s.client.DownloadImage(ctx, &img, destPath); err != nil {
			log.Warn("Failed to download bookmarked image", "id", img.ID, "error", err)
			continue
		}

		finalPath, ok := downloadedImagePath(favoritesDir, img.ID)
		if !ok {
			log.Warn("Download completed without a saved file", "id", img.ID)
			continue
		}

		metadata[img.ID] = &storage.ImageMeta{
			ID:           img.ID,
			Path:         finalPath,
			Width:        img.Width,
			Height:       img.Height,
			Title:        img.Title,
			Artist:       img.Artist,
			ArtistID:     img.ArtistID,
			Source:       "favorites",
			DownloadedAt: time.Now(),
		}
		downloaded++
	}

	if err := s.storage.SaveMetadata(metadata); err != nil {
		log.Error("Failed to save metadata", "error", err)
	}

	return downloaded
}

func downloadedImagePath(dir, imageID string) (string, bool) {
	for _, path := range []string{
		filepath.Join(dir, imageID+".jpg"),
		filepath.Join(dir, imageID+".png"),
	} {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func (s *Syncer) cleanupRemoved(ctx context.Context, remoteIDs map[string]struct{}) (int, error) {
	log := logger.WithComponent("bookmarks")
	metadata, err := s.storage.LoadMetadata()
	if err != nil {
		return 0, err
	}

	favoritesDir := s.storage.FavoritesDir()
	deleted := 0

	for id, meta := range metadata {
		if meta.Source != "favorites" {
			continue
		}
		if _, stillBookmarked := remoteIDs[id]; stillBookmarked {
			continue
		}

		log.Debug("Removing unbookmarked image", "id", id, "path", meta.Path)
		if rerr := os.Remove(meta.Path); rerr != nil && !os.IsNotExist(rerr) {
			log.Warn("Failed to remove unbookmarked image", "id", id, "error", rerr)
		}
		delete(metadata, id)

		if serr := s.storage.SaveMetadata(metadata); serr != nil {
			log.Error("Failed to save metadata after cleanup", "error", serr)
			return deleted, serr
		}

		deleted++
	}

	orphans, err := s.cleanupOrphanFiles(remoteIDs, favoritesDir, metadata)
	if err != nil {
		log.Warn("Failed to cleanup orphan files", "error", err)
	}
	deleted += orphans

	return deleted, nil
}

func (s *Syncer) cleanupOrphanFiles(remoteIDs map[string]struct{}, favoritesDir string, metadata map[string]*storage.ImageMeta) (int, error) {
	entries, err := os.ReadDir(favoritesDir)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		id := name[:len(name)-len(ext)]

		if _, inMeta := metadata[id]; inMeta {
			continue
		}
		if _, inRemote := remoteIDs[id]; inRemote {
			continue
		}

		path := filepath.Join(favoritesDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			continue
		}
		deleted++
	}

	return deleted, nil
}
