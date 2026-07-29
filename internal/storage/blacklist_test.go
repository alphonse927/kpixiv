package storage

import (
	"testing"
	"time"
)

func TestLoadBlacklistEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	blacklist, err := s.LoadBlacklist()
	if err != nil {
		t.Fatalf("LoadBlacklist() returned error: %v", err)
	}

	if blacklist == nil {
		t.Fatal("LoadBlacklist() returned nil")
	}

	if len(blacklist.IDs) != 0 {
		t.Fatalf("LoadBlacklist() IDs: got %d, want 0", len(blacklist.IDs))
	}
}

func TestExcludeWallpaper(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	history := &History{
		Current:   "22222",
		Images:    []string{"11111", "22222", "33333"},
		UpdatedAt: time.Now(),
	}
	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	if err = s.ExcludeWallpaper("22222"); err != nil {
		t.Fatalf("ExcludeWallpaper() returned error: %v", err)
	}

	blacklist, err := s.LoadBlacklist()
	if err != nil {
		t.Fatalf("LoadBlacklist() returned error: %v", err)
	}

	if len(blacklist.IDs) != 1 || blacklist.IDs[0] != "22222" {
		t.Fatalf("blacklist IDs: got %v, want [22222]", blacklist.IDs)
	}

	loadedHistory, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if loadedHistory.Current != "" {
		t.Fatalf("current history should be cleared, got %q", loadedHistory.Current)
	}

	for _, id := range loadedHistory.Images {
		if id == "22222" {
			t.Fatal("excluded wallpaper should be removed from history")
		}
	}

	if err = s.ExcludeWallpaper("22222"); err != nil {
		t.Fatalf("ExcludeWallpaper() second call returned error: %v", err)
	}

	blacklist, err = s.LoadBlacklist()
	if err != nil {
		t.Fatalf("LoadBlacklist() returned error: %v", err)
	}

	if len(blacklist.IDs) != 1 {
		t.Fatalf("blacklist should not duplicate IDs, got %v", blacklist.IDs)
	}
}
