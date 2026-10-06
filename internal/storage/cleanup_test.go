package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

	// "old" is no longer in use (the current wallpaper is "new"), so it is
	// eligible for cleanup. Wallpapers in use are covered by
	// TestCleanupKeepsWallpapersInUse.
	history := &History{Current: "new", Images: []string{"old"}, UpdatedAt: time.Now()}
	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	result, err := s.CleanupImagesOlderThanDays(7)
	if err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}

	if result.Removed < 1 {
		t.Fatalf("expected at least one removed image, got %d", result.Removed)
	}

	if result.FreedBytes < 3 {
		t.Fatalf("expected freed bytes for the removed image, got %d", result.FreedBytes)
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

	result, err := s.CleanupImagesOlderThanDays(0)
	if err != nil {
		t.Fatalf("CleanupImagesOlderThanDays(0) returned error: %v", err)
	}

	if result.Removed < 1 {
		t.Fatalf("expected all images to be removed, got %d", result.Removed)
	}

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("image should be removed by reset cleanup, stat error: %v", statErr)
	}
}

func TestCleanupKeepsWallpapersInUse(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	write := func(name string) string {
		p := filepath.Join(s.RankingDir(), name)
		if err = os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatalf("failed writing image: %v", err)
		}
		return p
	}

	current, monitor, other := write("cur.jpg"), write("mon.jpg"), write("old.jpg")
	old := time.Now().Add(-30 * 24 * time.Hour)
	meta := map[string]*ImageMeta{
		"cur": {ID: "cur", Path: current, DownloadedAt: old},
		"mon": {ID: "mon", Path: monitor, DownloadedAt: old},
		"old": {ID: "old", Path: other, DownloadedAt: old},
	}
	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}
	if err = s.AddToHistoryWithLimit("cur", 10); err != nil {
		t.Fatalf("AddToHistoryWithLimit() returned error: %v", err)
	}
	if err = s.AddToMonitorHistory("DP-1", "mon", 10); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}

	for _, days := range []int{7, 0} {
		if _, err = s.CleanupImagesOlderThanDays(days); err != nil {
			t.Fatalf("CleanupImagesOlderThanDays(%d) returned error: %v", days, err)
		}
		for _, p := range []string{current, monitor} {
			if _, statErr := os.Stat(p); statErr != nil {
				t.Fatalf("days=%d: in-use wallpaper %s was removed: %v", days, p, statErr)
			}
		}
	}
	if _, statErr := os.Stat(other); !os.IsNotExist(statErr) {
		t.Fatalf("unused wallpaper should be removed, stat error: %v", statErr)
	}
}
