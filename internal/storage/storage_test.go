package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
)

func TestNewCreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, ".local", "share", "kpixiv")
	rankingDir := filepath.Join(dataDir, "Ranking")
	downloadDir := filepath.Join(tmp, "downloads")

	s, err := New(tmp, downloadDir)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if s == nil {
		t.Fatal("New() returned nil")
	}

	if s.DataDir() != dataDir {
		t.Errorf("DataDir: got %s, want %s", s.DataDir(), dataDir)
	}

	if s.DownloadDir() != downloadDir {
		t.Errorf("DownloadDir: got %s, want %s", s.DownloadDir(), downloadDir)
	}

	if s.RankingDir() != rankingDir {
		t.Errorf("RankingDir: got %s, want %s", s.RankingDir(), rankingDir)
	}

	if _, err = os.Stat(rankingDir); err != nil {
		t.Errorf("Ranking directory not created: %v", err)
	}
}

func TestLoadPaginationStateEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	state, err := s.LoadPaginationState()
	if err != nil {
		t.Fatalf("LoadPaginationState() returned error: %v", err)
	}

	if state == nil {
		t.Fatal("LoadPaginationState() returned nil")
	}

	if state.Pages == nil {
		t.Error("Pages should not be nil for empty state")
	}
}

func TestSaveAndLoadPaginationState(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	original := &PaginationState{
		Pages: map[string]int{
			"daily_r18": 3,
			"weekly":    5,
		},
	}

	if err = s.SavePaginationState(original); err != nil {
		t.Fatalf("SavePaginationState() returned error: %v", err)
	}

	loaded, err := s.LoadPaginationState()
	if err != nil {
		t.Fatalf("LoadPaginationState() returned error: %v", err)
	}

	if loaded.Pages["daily_r18"] != 3 {
		t.Errorf("daily_r18: got %d, want 3", loaded.Pages["daily_r18"])
	}

	if loaded.Pages["weekly"] != 5 {
		t.Errorf("weekly: got %d, want 5", loaded.Pages["weekly"])
	}
}

func TestGetRankingPage(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	page, err := s.GetRankingPage(config.RankingDailyMode.String())
	if err != nil {
		t.Fatalf("GetRankingPage() returned error: %v", err)
	}

	if page != 1 {
		t.Errorf("GetRankingPage() for unknown key: got %d, want 1", page)
	}

	if err = s.SetRankingPage(config.RankingDailyMode.String(), 4); err != nil {
		t.Fatalf("SetRankingPage() returned error: %v", err)
	}

	page, err = s.GetRankingPage(config.RankingDailyMode.String())
	if err != nil {
		t.Fatalf("GetRankingPage() returned error: %v", err)
	}

	if page != 4 {
		t.Errorf("GetRankingPage() after SetRankingPage: got %d, want 4", page)
	}
}

func TestSetRankingPageClamping(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if err = s.SetRankingPage(config.RankingDailyMode.String(), 0); err != nil {
		t.Fatalf("SetRankingPage(0) returned error: %v", err)
	}

	page, err := s.GetRankingPage(config.RankingDailyMode.String())
	if err != nil {
		t.Fatalf("GetRankingPage() returned error: %v", err)
	}
	if page != 1 {
		t.Errorf("SetRankingPage(0) should clamp to 1: got %d", page)
	}
}

func TestLoadMetadataEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	meta, err := s.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() returned error: %v", err)
	}

	if meta == nil {
		t.Fatal("LoadMetadata() returned nil")
	}

	if len(meta) != 0 {
		t.Errorf("LoadMetadata() for non-existent file: got %d, want 0", len(meta))
	}
}

func TestSaveAndLoadMetadata(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	original := map[string]*ImageMeta{
		"12345": {
			ID:           "12345",
			Path:         "/path/to/image.jpg",
			Width:        1920,
			Height:       1080,
			Title:        "Test Artwork",
			Artist:       "Test Artist",
			ArtistID:     "99999",
			DownloadedAt: time.Now(),
		},
		"67890": {
			ID:           "67890",
			Path:         "/path/to/image2.png",
			Width:        2560,
			Height:       1440,
			Title:        "Another Artwork",
			Artist:       "Another Artist",
			ArtistID:     "88888",
			DownloadedAt: time.Now(),
		},
	}

	if err = s.SaveMetadata(original); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	loaded, err := s.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() returned error: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("LoadMetadata() count: got %d, want 2", len(loaded))
	}

	if loaded["12345"].Title != "Test Artwork" {
		t.Errorf("Loaded metadata for 12345: got %q", loaded["12345"].Title)
	}

	if loaded["67890"].Path != "/path/to/image2.png" {
		t.Errorf("Loaded metadata for 67890: got %q", loaded["67890"].Path)
	}
}

func TestAddImage(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	meta := &ImageMeta{
		ID:    "11111",
		Path:  "/some/path.jpg",
		Width: 1920,
	}

	if err = s.AddImage(meta); err != nil {
		t.Fatalf("AddImage() returned error: %v", err)
	}

	has, err := s.HasImage("11111")
	if err != nil {
		t.Fatalf("HasImage() returned error: %v", err)
	}
	if !has {
		t.Error("HasImage() after AddImage: got false, want true")
	}

	path, ok := s.GetImagePath("11111")
	if !ok {
		t.Error("GetImagePath() after AddImage: got false, want true")
	}
	if path != "/some/path.jpg" {
		t.Errorf("GetImagePath() path: got %q, want %q", path, "/some/path.jpg")
	}

	has, err = s.HasImage("99999")
	if err != nil {
		t.Fatalf("HasImage() returned error: %v", err)
	}
	if has {
		t.Error("HasImage() for non-existent ID: got true, want false")
	}
}

func TestLoadHistoryEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	history, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if history == nil {
		t.Fatal("LoadHistory() returned nil")
	}

	if len(history.Images) != 0 {
		t.Errorf("LoadHistory() for non-existent file: got %d, want 0", len(history.Images))
	}

	if history.Current != "" {
		t.Errorf("LoadHistory() Current: got %q, want empty", history.Current)
	}
}

func TestSaveAndLoadHistory(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	original := &History{
		Current:   "33333",
		Images:    []string{"11111", "22222", "33333"},
		UpdatedAt: time.Now(),
	}

	if err = s.SaveHistory(original); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	loaded, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if loaded.Current != "33333" {
		t.Errorf("LoadHistory() Current: got %q, want %q", loaded.Current, "33333")
	}

	if len(loaded.Images) != 3 {
		t.Errorf("LoadHistory() Images count: got %d, want 3", len(loaded.Images))
	}
}

func TestAddToHistory(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if err = s.AddToHistory("AAAAA"); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}

	history, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if history.Current != "AAAAA" {
		t.Errorf("AddToHistory() Current: got %q, want %q", history.Current, "AAAAA")
	}

	if err = s.AddToHistory("BBBBB"); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}

	history, err = s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if history.Current != "BBBBB" {
		t.Errorf("AddToHistory() second call Current: got %q, want %q", history.Current, "BBBBB")
	}

	if len(history.Images) != 2 {
		t.Errorf("AddToHistory() Images count: got %d, want 2", len(history.Images))
	}
}

func TestAddToHistoryTruncatesHistory(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	history := &History{
		Current:   "id50",
		Images:    make([]string, 50),
		UpdatedAt: time.Now(),
	}

	for i := range 50 {
		history.Images[i] = fmt.Sprintf("id%02d", i)
	}

	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	if err = s.AddToHistory("idNew"); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}

	loaded, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if len(loaded.Images) != 50 {
		t.Errorf("AddToHistory() should truncate to 50: got %d", len(loaded.Images))
	}

	if loaded.Images[0] != "id01" {
		t.Errorf("AddToHistory() kept wrong items: first item is %q, want id01", loaded.Images[0])
	}

	if loaded.Images[49] != "idNew" {
		t.Errorf("AddToHistory() last item: got %q, want idNew", loaded.Images[49])
	}
}

func TestGetCurrentWallpaper(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	current, err := s.GetCurrentWallpaper()
	if err != nil {
		t.Fatalf("GetCurrentWallpaper() returned error: %v", err)
	}
	if current != "" {
		t.Errorf("GetCurrentWallpaper() with no history: got %q, want empty", current)
	}

	if err = s.AddToHistory("XXXXX"); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}

	current, err = s.GetCurrentWallpaper()
	if err != nil {
		t.Fatalf("GetCurrentWallpaper() returned error: %v", err)
	}

	if current != "XXXXX" {
		t.Errorf("GetCurrentWallpaper() after AddToHistory: got %q, want %q", current, "XXXXX")
	}
}

func TestGetNextWallpaper(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	next, err := s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next != "" {
		t.Errorf("GetNextWallpaper() with no history: got %q, want empty", next)
	}

	history := &History{
		Current:   "BBBBB",
		Images:    []string{"AAAAA", "BBBBB", "CCCCC", "DDDDD"},
		UpdatedAt: time.Now(),
	}

	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	next, err = s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next != "CCCCC" {
		t.Errorf("GetNextWallpaper() at position 1: got %q, want %q", next, "CCCCC")
	}

	if err = s.AddToHistory("CCCCC"); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}
	next, err = s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next != "DDDDD" {
		t.Errorf("GetNextWallpaper() at position 2: got %q, want %q", next, "DDDDD")
	}
}

func TestGetNextWallpaperAtCurrent(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	history := &History{
		Current:   "AAAAA",
		Images:    []string{"AAAAA", "BBBBB", "CCCCC"},
		UpdatedAt: time.Now(),
	}

	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	next, err := s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next != "BBBBB" {
		t.Errorf("GetNextWallpaper() at last item: got %q, want %q", next, "BBBBB")
	}
}

func TestGetNextWallpaperEmptyHistory(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	history := &History{
		Current:   "",
		Images:    []string{},
		UpdatedAt: time.Now(),
	}

	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	next, err := s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next != "" {
		t.Errorf("GetNextWallpaper() with empty history: got %q, want empty", next)
	}
}

func TestCleanupImagesOlderThanDays(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	oldPath := filepath.Join(s.RankingDir(), "old.jpg")
	newPath := filepath.Join(s.RankingDir(), "new.jpg")
	if err = os.WriteFile(oldPath, []byte("old"), 0600); err != nil {
		t.Fatalf("failed writing old image: %v", err)
	}
	if err = os.WriteFile(newPath, []byte("new"), 0600); err != nil {
		t.Fatalf("failed writing new image: %v", err)
	}

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err = os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed setting old file times: %v", err)
	}

	meta := map[string]*ImageMeta{
		"old": {ID: "old", Path: oldPath, DownloadedAt: oldTime},
		"new": {ID: "new", Path: newPath, DownloadedAt: time.Now()},
	}
	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	history := &History{Current: "old", Images: []string{"old", "new"}, UpdatedAt: time.Now()}
	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	removed, err := s.CleanupImagesOlderThanDays(7)
	if err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}
	if removed < 1 {
		t.Fatalf("expected at least one removed image, got %d", removed)
	}

	if _, statErr := os.Stat(oldPath); !os.IsNotExist(statErr) {
		t.Fatalf("old file should be removed, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Fatalf("new file should remain, got stat error: %v", statErr)
	}

	images, err := s.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() returned error: %v", err)
	}
	if _, exists := images["old"]; exists {
		t.Fatalf("old metadata entry should be removed")
	}
	if _, exists := images["new"]; !exists {
		t.Fatalf("new metadata entry should remain")
	}
}

func TestCleanupImagesOlderThanDaysResetRemovesAll(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	path := filepath.Join(s.RankingDir(), "all.jpg")
	if err = os.WriteFile(path, []byte("all"), 0600); err != nil {
		t.Fatalf("failed writing image: %v", err)
	}
	meta := map[string]*ImageMeta{
		"all": {ID: "all", Path: path, DownloadedAt: time.Now()},
	}
	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	removed, err := s.CleanupImagesOlderThanDays(0)
	if err != nil {
		t.Fatalf("CleanupImagesOlderThanDays(0) returned error: %v", err)
	}
	if removed < 1 {
		t.Fatalf("expected all images to be removed, got %d", removed)
	}

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("image should be removed by reset cleanup, stat error: %v", statErr)
	}
}
