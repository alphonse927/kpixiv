package fetcher

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

type FetchResult struct {
	Total      int
	Filtered   int
	Downloaded int
	Skipped    int
	Failed     int
	QueuedIDs  []string
	NextPage   int
}

type Fetcher struct {
	cfg     *config.Config
	storage *storage.Storage
	client  pixiv.ImageClient
	page    int
	mode    string
}

// NewFetcher creates a fetcher bound to config, storage, and pixiv client.
func NewFetcher(cfg *config.Config, st *storage.Storage, client pixiv.ImageClient) *Fetcher {
	return &Fetcher{
		cfg:     cfg,
		storage: st,
		client:  client,
		page:    1,
		mode:    cfg.Pixiv.Ranking.String(),
	}
}

// Fetch downloads ranking images, updates metadata, and appends queue candidates.
func (f *Fetcher) Fetch(ctx context.Context) (*FetchResult, error) {
	log := logger.WithComponent("fetcher")

	rMode := f.cfg.Pixiv.Ranking.String()
	pageKey := fmt.Sprintf("%s:%t", f.cfg.Pixiv.Ranking, f.cfg.Pixiv.R18)

	images, nextPage, err := f.client.FetchRanking(ctx, rMode, f.page, f.cfg.Pixiv.R18)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rankings: %w", err)
	}

	if err = f.storage.SetRankingPage(pageKey, nextPage); err != nil {
		log.Warn("Failed to save next ranking page", "error", err)
	}

	result := &FetchResult{
		Total:    len(images),
		NextPage: nextPage,
	}

	blacklist, blErr := f.storage.LoadBlacklistSet()
	if blErr != nil {
		return nil, fmt.Errorf("failed to load blacklist: %w", blErr)
	}

	filteredImages := f.filterImages(images, blacklist)
	result.Filtered = len(filteredImages)

	log.Debug("Filtered images", "count", result.Filtered, "minWidth", f.cfg.Pixiv.MinWidth, "minHeight", f.cfg.Pixiv.MinHeight)

	pending, metadata, err := f.prepareDownloads(filteredImages)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare downloads: %w", err)
	}

	downloadedIDs := f.downloadAndSave(ctx, pending, metadata)
	availableIDs := f.collectAvailableIDs(filteredImages, downloadedIDs, metadata)
	result.Downloaded = len(downloadedIDs)
	result.Skipped = len(filteredImages) - len(pending)
	result.Failed = len(pending) - len(downloadedIDs)
	result.QueuedIDs = availableIDs

	log.Info("Fetch summary", "downloaded", result.Downloaded, "skipped", result.Skipped, "failed", result.Failed)

	q := storage.NewQueue(f.storage.StateDir())
	if err = q.Load(); err != nil {
		log.Warn("Failed to load queue", "error", err)
	}

	if len(availableIDs) > 0 {
		if err = q.AppendRandom(availableIDs); err != nil {
			log.Warn("Failed to append to queue", "error", err)
		}
	}

	f.page = nextPage
	return result, nil
}

func (f *Fetcher) filterImages(images []pixiv.Image, blacklist map[string]struct{}) []pixiv.Image {
	orientation := f.fetchOrientation()
	var filtered []pixiv.Image
	for _, img := range images {
		if _, excluded := blacklist[img.ID]; excluded {
			continue
		}

		if img.Width < f.cfg.Pixiv.MinWidth || img.Height < f.cfg.Pixiv.MinHeight {
			continue
		}

		if !orientation.Matches(img.Width, img.Height) {
			continue
		}

		filtered = append(filtered, img)
	}

	return filtered
}

// fetchOrientation returns the orientation used when downloading new images.
// Single-monitor mode uses the global wallpaper orientation. Multi-monitor
// mode keeps the legacy behavior: downloads are restricted to landscape only
// when every configured monitor requires landscape, otherwise anything is
// accepted, and per-monitor filtering happens during rotation.
func (f *Fetcher) fetchOrientation() config.WallpaperOrientation {
	if !f.cfg.Wallpaper.MultiMonitorEnabled {
		return f.cfg.Wallpaper.Orientation
	}

	for _, monitor := range f.cfg.Wallpaper.Monitors {
		if monitor.Orientation != config.WallpaperLandscapeOrientation {
			return config.WallpaperAnyOrientation
		}
	}

	return config.WallpaperLandscapeOrientation
}

func (f *Fetcher) prepareDownloads(filteredImages []pixiv.Image) ([]pixiv.Image, map[string]*storage.ImageMeta, error) {
	rankingDir := f.storage.RankingDir()
	metadata, err := f.storage.LoadMetadata()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	var pending []pixiv.Image
	for _, img := range filteredImages {
		if f.metadataAlreadyExists(&img, metadata) {
			continue
		}

		destPath := filepath.Join(rankingDir, img.ID+".jpg")
		altPath := filepath.Join(rankingDir, img.ID+".png")
		if f.checkAndSaveExisting(destPath, altPath, &img, metadata) {
			continue
		}

		pending = append(pending, img)
	}

	return pending, metadata, nil
}

func (f *Fetcher) metadataAlreadyExists(img *pixiv.Image, metadata map[string]*storage.ImageMeta) bool {
	existing, ok := metadata[img.ID]
	if !ok {
		return false
	}

	if _, err := os.Stat(existing.Path); err != nil {
		return false
	}

	if existing.Source == f.mode {
		return true
	}

	existing.Source = f.mode
	existing.Rank = img.Rank
	existing.DownloadedAt = time.Now()
	return true
}

func (f *Fetcher) checkAndSaveExisting(destPath, altPath string, img *pixiv.Image, metadata map[string]*storage.ImageMeta) bool {
	if _, err := os.Stat(destPath); err == nil {
		metadata[img.ID] = &storage.ImageMeta{
			ID:           img.ID,
			Path:         destPath,
			Width:        img.Width,
			Height:       img.Height,
			Title:        img.Title,
			Artist:       img.Artist,
			ArtistID:     img.ArtistID,
			Rank:         img.Rank,
			Source:       f.mode,
			DownloadedAt: time.Now(),
		}

		return true
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
			Rank:         img.Rank,
			Source:       f.mode,
			DownloadedAt: time.Now(),
		}

		return true
	}

	return false
}

func (f *Fetcher) downloadAndSave(ctx context.Context, pending []pixiv.Image, metadata map[string]*storage.ImageMeta) []string {
	log := logger.WithComponent("fetcher")
	rankingDir := f.storage.RankingDir()

	var downloadedIDs []string
	for _, img := range pending {
		destPath := filepath.Join(rankingDir, img.ID+".jpg")
		if err := f.client.DownloadImage(ctx, &img, destPath); err != nil {
			log.Warn("Failed to download image", "id", img.ID, "error", err)
			continue
		}

		finalPath, ok := downloadedImagePath(rankingDir, img.ID)
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
			Rank:         img.Rank,
			Source:       f.mode,
			DownloadedAt: time.Now(),
		}
		downloadedIDs = append(downloadedIDs, img.ID)

		if err := f.storage.GenerateThumbnail(finalPath, img.ID); err != nil {
			log.Warn("Failed to generate thumbnail", "id", img.ID, "error", err)
		}
	}

	if err := f.storage.SaveMetadata(metadata); err != nil {
		log.Error("Failed to save metadata", "error", err)
	}

	return downloadedIDs
}

func downloadedImagePath(rankingDir, imageID string) (string, bool) {
	for _, path := range []string{
		filepath.Join(rankingDir, imageID+".jpg"),
		filepath.Join(rankingDir, imageID+".png"),
	} {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return "", false
}

func (f *Fetcher) collectAvailableIDs(filteredImages []pixiv.Image, downloadedIDs []string, metadata map[string]*storage.ImageMeta) []string {
	available := make(map[string]bool)

	for _, id := range downloadedIDs {
		available[id] = true
	}

	for _, img := range filteredImages {
		if meta, ok := metadata[img.ID]; ok && meta.Path != "" {
			if _, err := os.Stat(meta.Path); err == nil {
				available[img.ID] = true
			}
		}
	}

	ids := make([]string, 0, len(available))
	for id := range available {
		ids = append(ids, id)
	}

	return ids
}

// GetPage returns the current ranking page pointer used by the fetcher.
func (f *Fetcher) GetPage() int {
	return f.page
}

// SetPage sets the current ranking page pointer used by the fetcher.
func (f *Fetcher) SetPage(page int) {
	f.page = page
}

// LoadPage loads the persisted ranking page pointer from storage.
func (f *Fetcher) LoadPage() error {
	pageKey := fmt.Sprintf("%s:%t", f.cfg.Pixiv.Ranking, f.cfg.Pixiv.R18)
	page, err := f.storage.GetRankingPage(pageKey)
	if err != nil {
		return fmt.Errorf("failed to load ranking page: %w", err)
	}

	f.page = page
	return nil
}

// DryRun fetches rankings and filtering stats without downloading files.
func (f *Fetcher) DryRun(ctx context.Context) (*FetchResult, error) {
	log := logger.WithComponent("fetcher")

	rMode := f.cfg.Pixiv.Ranking.String()

	images, nextPage, err := f.client.FetchRanking(ctx, rMode, f.page, f.cfg.Pixiv.R18)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rankings: %w", err)
	}

	result := &FetchResult{
		Total:    len(images),
		NextPage: nextPage,
	}

	blacklist, err := f.storage.LoadBlacklistSet()
	if err != nil {
		return nil, fmt.Errorf("failed to load blacklist: %w", err)
	}

	filteredImages := f.filterImages(images, blacklist)
	result.Filtered = len(filteredImages)
	result.Skipped = result.Filtered

	log.Info("Dry-run mode: skipping downloads", "candidates", result.Filtered)
	fmt.Println("Fetch dry-run complete!")
	fmt.Printf("Total: %d, Filtered: %d, Downloaded: 0, Skipped: %d\n", result.Total, result.Filtered, result.Skipped)

	return result, nil
}
