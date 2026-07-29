package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/slices"
)

type ImageMeta struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	Title        string    `json:"title"`
	Artist       string    `json:"artist"`
	ArtistID     string    `json:"artist_id"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Rank         int       `json:"rank"`
	Source       string    `json:"source"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type History struct {
	Current   string            `json:"current"`
	Images    []string          `json:"images"`
	Monitors  map[string]string `json:"monitors,omitempty"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type PaginationState struct {
	Pages            map[string]int `json:"pages"`
	LastBookmarkPage string         `json:"last_bookmark_page,omitempty"`
	BookmarkComplete bool           `json:"bookmark_complete,omitempty"`
}

type Blacklist struct {
	IDs       []string  `json:"ids"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Storage struct {
	dataDir      string
	downloadDir  string
	downloadPath string
	homeDir      string
	stateDir     string
}

var invalidFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

// New initializes storage directories and returns a storage handle.
func New(homeDir, downloadPath string) (*Storage, error) {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir() //nolint: errcheck // ignore error
	}

	dataDir := filepath.Join(homeDir, ".local", "share", "kpixiv")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}

	stateDir := filepath.Join(homeDir, ".local", "state", "kpixiv")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		return nil, err
	}

	rankingDir := filepath.Join(dataDir, "Ranking")
	if err := os.MkdirAll(rankingDir, 0750); err != nil {
		return nil, err
	}

	bookmarksDir := filepath.Join(dataDir, "Bookmarks")
	if err := os.MkdirAll(bookmarksDir, 0750); err != nil {
		return nil, err
	}

	thumbnailsDir := filepath.Join(dataDir, "Thumbnails")
	if err := os.MkdirAll(thumbnailsDir, 0750); err != nil {
		return nil, err
	}

	downloadDir := resolveDownloadDir(homeDir, downloadPath)

	if err := os.MkdirAll(downloadDir, 0750); err != nil {
		return nil, err
	}

	return &Storage{
		dataDir:      dataDir,
		downloadDir:  downloadDir,
		downloadPath: downloadDir,
		homeDir:      homeDir,
		stateDir:     stateDir,
	}, nil
}

func resolveDownloadDir(homeDir, downloadPath string) string {
	trimmed := strings.TrimSpace(downloadPath)
	if trimmed == "" {
		return filepath.Join(homeDir, "Pictures", "KPixiv")
	}

	if trimmed == "~" {
		return homeDir
	}

	if strings.HasPrefix(trimmed, "~/") {
		return filepath.Join(homeDir, strings.TrimPrefix(trimmed, "~/"))
	}

	return trimmed
}

// DataDir returns the base data directory.
func (s *Storage) DataDir() string {
	return s.dataDir
}

// StateDir returns the base state directory.
func (s *Storage) StateDir() string {
	return s.stateDir
}

// DownloadDir returns the configured wallpaper download directory.
func (s *Storage) DownloadDir() string {
	return s.downloadDir
}

// CopyImageToDownloadDir copies a known image into the configured download directory.
func (s *Storage) CopyImageToDownloadDir(id string) (string, error) {
	sourcePath, ok := s.GetImagePath(id)
	if !ok {
		return "", fmt.Errorf("artwork %s not found on disk", id)
	}

	meta, _ := s.lookupImageMeta(id)
	filename := s.downloadFilename(id, sourcePath, meta)
	destPath := filepath.Join(s.downloadDir, filename)

	// Validating if the same file
	if filepath.Clean(sourcePath) == filepath.Clean(destPath) {
		return destPath, nil
	}

	if err := copyFileAtomic(sourcePath, destPath); err != nil {
		return "", err
	}

	return destPath, nil
}

// RankingDir returns the ranking image directory.
func (s *Storage) RankingDir() string {
	return filepath.Join(s.dataDir, "Ranking")
}

// BookmarksDir returns the bookmark image directory.
func (s *Storage) BookmarksDir() string {
	return filepath.Join(s.dataDir, "Bookmarks")
}

// ThumbnailDir returns the thumbnail image directory.
func (s *Storage) ThumbnailDir() string {
	return filepath.Join(s.dataDir, "Thumbnails")
}

// ThumbnailPath returns the expected thumbnail path for a given image ID.
func (s *Storage) ThumbnailPath(id string) string {
	return filepath.Join(s.ThumbnailDir(), id+".jpg")
}

// GenerateThumbnail scales a source image to 140px wide and saves it as a JPEG
// thumbnail. Skips if the thumbnail already exists.
func (s *Storage) GenerateThumbnail(srcPath, id string) error {
	dstPath := s.ThumbnailPath(id)
	if _, err := os.Stat(dstPath); err == nil {
		return nil
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}

	defer f.Close() //nolint:errcheck // read-only file

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	thumb := scaleImage(src, 140)
	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create thumbnail: %w", err)
	}

	defer out.Close() //nolint:errcheck // flushed before close

	if err = jpeg.Encode(out, thumb, &jpeg.Options{Quality: 75}); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}

	return nil
}

func scaleImage(src image.Image, maxWidth int) image.Image {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= maxWidth {
		return src
	}

	newH := h * maxWidth / w
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, newH))
	for y := range newH {
		for x := range maxWidth {
			srcX := x * w / maxWidth
			srcY := y * h / newH
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

// FavoritesMetadataPath returns the favorite metadata JSON path (same as the main metadata).
func (s *Storage) FavoritesMetadataPath() string {
	return s.MetadataPath()
}

// MetadataPath returns the metadata JSON file path.
func (s *Storage) MetadataPath() string {
	return filepath.Join(s.stateDir, "metadata.json")
}

// HistoryPath returns the history JSON file path.
func (s *Storage) HistoryPath() string {
	return filepath.Join(s.stateDir, "history.json")
}

// PaginationPath returns the pagination JSON file path.
func (s *Storage) PaginationPath() string {
	return filepath.Join(s.stateDir, "pagination.json")
}

// BlacklistPath returns the blacklist JSON file path.
func (s *Storage) BlacklistPath() string {
	return filepath.Join(s.stateDir, "blacklist.json")
}

// LoadBlacklist reads the excluded wallpaper IDs from the disk.
func (s *Storage) LoadBlacklist() (*Blacklist, error) {
	path := s.BlacklistPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Blacklist{IDs: []string{}, UpdatedAt: time.Now()}, nil
		}
		return nil, err
	}

	var blacklist Blacklist
	if err = json.Unmarshal(data, &blacklist); err != nil {
		return nil, err
	}

	if blacklist.IDs == nil {
		blacklist.IDs = []string{}
	}

	return &blacklist, nil
}

// SaveBlacklist writes the excluded wallpaper IDs to the disk.
func (s *Storage) SaveBlacklist(blacklist *Blacklist) error {
	if blacklist.IDs == nil {
		blacklist.IDs = []string{}
	}

	blacklist.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(blacklist, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.BlacklistPath(), data, 0600)
}

// LoadBlacklistSet reads the blacklist and returns it as a set.
func (s *Storage) LoadBlacklistSet() (map[string]struct{}, error) {
	blacklist, err := s.LoadBlacklist()
	if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{}, len(blacklist.IDs))
	for _, id := range blacklist.IDs {
		ids[id] = struct{}{}
	}

	return ids, nil
}

// ExcludeWallpaper persists an image ID in the blacklist and removes it from history.
func (s *Storage) ExcludeWallpaper(imageID string) error {
	blacklist, err := s.LoadBlacklist()
	if err != nil {
		return err
	}

	blacklist.IDs = slices.Unique(append(blacklist.IDs, imageID))
	if err = s.SaveBlacklist(blacklist); err != nil {
		return err
	}

	history, err := s.LoadHistory()
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(history.Images))
	for _, id := range history.Images {
		if id == imageID {
			continue
		}
		filtered = append(filtered, id)
	}

	history.Images = filtered
	if history.Current == imageID {
		history.Current = ""
	}

	return s.SaveHistory(history)
}

// LoadPaginationState reads persisted ranking pagination state.
func (s *Storage) LoadPaginationState() (*PaginationState, error) {
	path := s.PaginationPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PaginationState{Pages: map[string]int{}}, nil
		}
		return nil, err
	}

	var state PaginationState
	if err = json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Pages == nil {
		state.Pages = map[string]int{}
	}

	return &state, nil
}

// SavePaginationState writes ranking pagination state.
func (s *Storage) SavePaginationState(state *PaginationState) error {
	if state.Pages == nil {
		state.Pages = map[string]int{}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.PaginationPath(), data, 0600)
}

// GetRankingPage returns the persisted page for a ranking key.
func (s *Storage) GetRankingPage(key string) (int, error) {
	state, err := s.LoadPaginationState()
	if err != nil {
		return 1, err
	}

	page, ok := state.Pages[key]
	if !ok || page < 1 {
		return 1, nil
	}

	return page, nil
}

// SetRankingPage persists the page for a ranking key.
func (s *Storage) SetRankingPage(key string, page int) error {
	if page < 1 {
		page = 1
	}

	state, err := s.LoadPaginationState()
	if err != nil {
		return err
	}

	state.Pages[key] = page
	return s.SavePaginationState(state)
}

// GetBookmarkPagination returns the bookmark sync cursor state.
func (s *Storage) GetBookmarkPagination() (lastPageURL string, complete bool, err error) {
	state, err := s.LoadPaginationState()
	if err != nil {
		return "", false, err
	}

	return state.LastBookmarkPage, state.BookmarkComplete, nil
}

// SetBookmarkPagination persists the bookmark sync cursor state.
func (s *Storage) SetBookmarkPagination(lastPageURL string, complete bool) error {
	state, err := s.LoadPaginationState()
	if err != nil {
		return err
	}

	state.LastBookmarkPage = lastPageURL
	state.BookmarkComplete = complete
	return s.SavePaginationState(state)
}

// BookmarkPath returns the bookmark JSON file path.
func (s *Storage) BookmarkPath() string {
	return filepath.Join(s.stateDir, "bookmarks.json")
}

// LoadBookmarks reads bookmarked artwork IDs from the disk.
func (s *Storage) LoadBookmarks() (map[string]struct{}, error) {
	bd, err := s.loadBookmarkData()
	if err != nil {
		return nil, err
	}

	bookmarks := make(map[string]struct{}, len(bd.IDs))
	for _, id := range bd.IDs {
		bookmarks[id] = struct{}{}
	}

	return bookmarks, nil
}

// SaveBookmarks writes the bookmark file.
func (s *Storage) SaveBookmarks(ids []string) error {
	if ids == nil {
		ids = []string{}
	}

	bd := NewBookmarkData(ids)
	data, err := json.MarshalIndent(bd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.BookmarkPath(), data, 0600)
}

// IsArtworkBookmarked checks whether an artwork ID is in the bookmark set.
func (s *Storage) IsArtworkBookmarked(id string) (bool, error) {
	bookmarks, err := s.LoadBookmarks()
	if err != nil {
		return false, err
	}

	_, ok := bookmarks[id]
	return ok, nil
}

// AddBookmark persists an artwork ID as bookmarked locally.
func (s *Storage) AddBookmark(id string) error {
	bookmarks, err := s.LoadBookmarks()
	if err != nil {
		return err
	}

	if _, exists := bookmarks[id]; exists {
		return nil
	}

	ids := make([]string, 0, len(bookmarks)+1)
	for bid := range bookmarks {
		ids = append(ids, bid)
	}

	ids = append(ids, id)
	bd := NewBookmarkData(ids)

	data, err := json.MarshalIndent(bd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.BookmarkPath(), data, 0600)
}

// AddBookmarks bulk-imports multiple IDs into the bookmark file (no duplicates).
func (s *Storage) AddBookmarks(ids []string) error {
	bd, err := s.loadBookmarkData()
	if err != nil {
		return err
	}

	all := make([]string, 0, len(bd.IDs)+len(ids))
	all = append(all, bd.IDs...)
	all = append(all, ids...)
	bd.IDs = slices.Unique(all)
	bd.LastUpdate = time.Now()

	data, err := json.MarshalIndent(bd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.BookmarkPath(), data, 0600)
}

func (s *Storage) loadBookmarkData() (*BookmarkData, error) {
	path := s.BookmarkPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return new(NewBookmarkData([]string{})), nil
		}
		return nil, err
	}

	var bd BookmarkData
	if err = json.Unmarshal(data, &bd); err == nil && bd.IDs != nil {
		return &bd, nil
	}

	var ids []string
	if err = json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}

	bd = NewBookmarkData(ids)
	return &bd, nil
}

// LoadMetadata reads image metadata from the disk.
func (s *Storage) LoadMetadata() (map[string]*ImageMeta, error) {
	path := s.MetadataPath()
	data, rfErr := os.ReadFile(path)
	if rfErr != nil {
		if os.IsNotExist(rfErr) {
			return make(map[string]*ImageMeta), nil
		}
		return nil, rfErr
	}

	var images map[string]*ImageMeta
	if err := json.Unmarshal(data, &images); err != nil {
		return nil, err
	}

	return images, nil
}

// SaveMetadata writes image metadata to disk.
func (s *Storage) SaveMetadata(images map[string]*ImageMeta) error {
	data, err := json.MarshalIndent(images, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.MetadataPath(), data, 0600)
}

// GetImagePath returns an image path from metadata, ranking dir, or bookmarks dir.
func (s *Storage) GetImagePath(id string) (string, bool) {
	images, err := s.LoadMetadata()
	if err != nil {
		return "", false
	}

	meta, exists := images[id]
	if exists && meta.Path != "" {
		if _, err := os.Stat(meta.Path); err == nil {
			return meta.Path, true
		}
	}

	// Fall back to searching directories
	if path, ok := s.findImageInRankingDir(id); ok {
		return path, true
	}

	return s.findImageInBookmarksDir(id)
}

func (s *Storage) lookupImageMeta(id string) (*ImageMeta, bool) {
	images, err := s.LoadMetadata()
	if err != nil {
		return nil, false
	}

	meta, ok := images[id]
	return meta, ok
}

// LoadHistory reads wallpaper history from the disk.
func (s *Storage) LoadHistory() (*History, error) {
	path := s.HistoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &History{
				Images:    []string{},
				Monitors:  map[string]string{},
				UpdatedAt: time.Now(),
			}, nil
		}

		return nil, err
	}

	var history History
	if err = json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	if history.Monitors == nil {
		history.Monitors = map[string]string{}
	}

	return &history, nil
}

// SaveHistory writes wallpaper history to disk.
func (s *Storage) SaveHistory(history *History) error {
	history.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.HistoryPath(), data, 0600)
}

func (s *Storage) LoadMonitorHistory() (map[string]string, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return nil, err
	}

	return history.Monitors, nil
}

func (s *Storage) SaveMonitorHistory(monitors map[string]string, historyLimit int) error {
	history, err := s.LoadHistory()
	if err != nil {
		return err
	}

	if history.Monitors == nil {
		history.Monitors = map[string]string{}
	}

	for screenID, imageID := range monitors {
		if oldID, exists := history.Monitors[screenID]; exists && oldID != "" && oldID != imageID {
			history.Images = append(history.Images, oldID)
		}

		history.Monitors[screenID] = imageID
	}

	trimHistory(history, historyLimit)
	return s.SaveHistory(history)
}

func (s *Storage) AddToMonitorHistory(screenID, imageID string, historyLimit int) error {
	monitors, err := s.LoadMonitorHistory()
	if err != nil {
		return err
	}

	monitors[screenID] = imageID

	if err = s.SaveMonitorHistory(monitors, historyLimit); err != nil {
		return err
	}

	history, err := s.LoadHistory()
	if err != nil {
		return err
	}

	if history.Current != "" && history.Current != imageID {
		history.Images = append(history.Images, history.Current)
	}

	history.Current = imageID
	trimHistory(history, historyLimit)

	return s.SaveHistory(history)
}

// AddToHistoryWithLimit updates current wallpaper and trims history length.
func (s *Storage) AddToHistoryWithLimit(imageID string, historyLimit int) error {
	history, err := s.LoadHistory()
	if err != nil {
		return err
	}

	if history.Current != "" && history.Current != imageID {
		history.Images = append(history.Images, history.Current)
	}

	history.Current = imageID
	trimHistory(history, historyLimit)

	return s.SaveHistory(history)
}

func trimHistory(history *History, historyLimit int) bool {
	limit := max(historyLimit, 1)
	if len(history.Images) <= limit {
		return false
	}

	history.Images = history.Images[len(history.Images)-limit:]
	return true
}

// GetCurrentWallpaper returns the current wallpaper ID.
func (s *Storage) GetCurrentWallpaper() (string, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return "", err
	}

	return history.Current, nil
}

// GetNextWallpaper returns the next wallpaper ID from the queue.
func (s *Storage) GetNextWallpaper() (string, error) {
	q := NewQueue(s.stateDir)
	if err := q.Load(); err != nil {
		return "", err
	}

	nextID, ok := q.Peek()
	if !ok {
		return "", nil
	}

	return nextID, nil
}

func (s *Storage) findImageInRankingDir(id string) (string, bool) {
	return findImageInDir(s.RankingDir(), id)
}

func (s *Storage) findImageInBookmarksDir(id string) (string, bool) {
	return findImageInDir(s.BookmarksDir(), id)
}

func findImageInDir(dir, id string) (string, bool) {
	for _, ext := range []string{".jpg", ".jpeg", ".png"} {
		path := filepath.Join(dir, id+ext)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return "", false
}

func (s *Storage) downloadFilename(id, sourcePath string, meta *ImageMeta) string {
	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".jpg"
	}

	if meta == nil || strings.TrimSpace(meta.Title) == "" {
		return id + ext
	}

	title := invalidFilenameChars.ReplaceAllString(strings.TrimSpace(meta.Title), " ")
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return id + ext
	}

	return fmt.Sprintf("%s - %s%s", id, title, ext)
}

func copyFileAtomic(sourcePath, destPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source artwork: %w", err)
	}
	defer source.Close() //nolint:errcheck // deferred close on best-effort basis

	if err = os.MkdirAll(filepath.Dir(destPath), 0750); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), ".copy-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary download file: %w", err)
	}

	tmpPath := tmpFile.Name()
	defer tmpFile.Close()    //nolint:errcheck // deferred close on best-effort basis
	defer os.Remove(tmpPath) //nolint:errcheck // deferred cleanup on best-effort basis

	if _, err = io.Copy(tmpFile, source); err != nil {
		return fmt.Errorf("failed to copy artwork: %w", err)
	}

	if err = tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to flush copied artwork: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close copied artwork: %w", err)
	}

	if err = os.Rename(tmpPath, destPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("failed to finalize copied artwork: %w", err)
	}

	return nil
}

// CleanupImagesOlderThanDays removes old images and syncs metadata/history.
func (s *Storage) CleanupImagesOlderThanDays(days int) (int, error) {
	images, err := s.LoadMetadata()
	if err != nil {
		return 0, err
	}

	cutoff, removeAll := cleanupCutoff(days)
	removedIDs, removedFiles, removedFromMetadata, mrErr := s.cleanupMetadata(images, cutoff, removeAll)
	if mrErr != nil {
		return 0, mrErr
	}

	if err = s.SaveMetadata(images); err != nil {
		return 0, err
	}

	removedFromRanking, crErr := s.cleanupRankingFiles(cutoff, removeAll, removedFiles)
	if crErr != nil {
		return 0, crErr
	}

	if err = s.cleanupHistory(removedIDs); err != nil {
		return 0, err
	}

	if err = s.cleanupQueue(removedIDs); err != nil {
		return removedFromMetadata + removedFromRanking, err
	}

	return removedFromMetadata + removedFromRanking, nil
}

func cleanupCutoff(days int) (time.Time, bool) {
	removeAll := days <= 0
	if removeAll {
		return time.Now(), true
	}

	return time.Now().Add(-time.Duration(days) * 24 * time.Hour), false
}

func (s *Storage) cleanupMetadata(images map[string]*ImageMeta, cutoff time.Time, removeAll bool) (map[string]struct{}, map[string]struct{}, int, error) {
	removedIDs := make(map[string]struct{})
	removedFiles := make(map[string]struct{})
	removedCount := 0

	for id, meta := range images {
		if meta.Source == "bookmarks" {
			continue
		}

		if !removeAll && !meta.DownloadedAt.Before(cutoff) {
			continue
		}

		if meta.Path != "" {
			if rmErr := os.Remove(meta.Path); rmErr != nil && !os.IsNotExist(rmErr) {
				return nil, nil, removedCount, fmt.Errorf("failed to remove image file %s: %w", meta.Path, rmErr)
			}
			removedFiles[meta.Path] = struct{}{}
		}

		delete(images, id)
		removedIDs[id] = struct{}{}
		removedCount++
	}

	return removedIDs, removedFiles, removedCount, nil
}

func (s *Storage) cleanupRankingFiles(cutoff time.Time, removeAll bool, removedFiles map[string]struct{}) (int, error) {
	rankingEntries, readErr := os.ReadDir(s.RankingDir())
	if readErr != nil {
		return 0, fmt.Errorf("failed to read ranking directory: %w", readErr)
	}

	removedCount := 0
	for _, entry := range rankingEntries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(s.RankingDir(), entry.Name())
		if _, alreadyRemoved := removedFiles[path]; alreadyRemoved {
			continue
		}

		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}

		if !removeAll && !info.ModTime().Before(cutoff) {
			continue
		}

		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return removedCount, fmt.Errorf("failed to remove ranking image %s: %w", path, rmErr)
		}

		removedCount++
	}

	return removedCount, nil
}

func (s *Storage) cleanupHistory(removedIDs map[string]struct{}) error {
	history, err := s.LoadHistory()
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(history.Images))
	for _, id := range history.Images {
		if _, removed := removedIDs[id]; removed {
			continue
		}
		filtered = append(filtered, id)
	}

	history.Images = filtered
	if _, removed := removedIDs[history.Current]; removed {
		history.Current = ""
	}

	return s.SaveHistory(history)
}

func (s *Storage) cleanupQueue(removedIDs map[string]struct{}) error {
	q := NewQueue(s.stateDir)
	if err := q.Load(); err != nil {
		return err
	}

	if q.IsEmpty() {
		return nil
	}

	for id := range removedIDs {
		if err := q.Remove(id); err != nil {
			return err
		}
	}

	return nil
}
