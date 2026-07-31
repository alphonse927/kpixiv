package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorHistoryRoundTrip(t *testing.T) {
	s, err := New(t.TempDir(), filepath.Join(t.TempDir(), "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if err = s.AddToMonitorHistory("1", "wallpaper-a", 100); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}
	if err = s.AddToMonitorHistory("2", "wallpaper-b", 100); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}

	monitors, err := s.LoadMonitorHistory()
	if err != nil {
		t.Fatalf("LoadMonitorHistory() returned error: %v", err)
	}
	if monitors["1"] != "wallpaper-a" || monitors["2"] != "wallpaper-b" {
		t.Fatalf("unexpected monitor history: %#v", monitors)
	}
	mainHistory, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if mainHistory.Monitors["1"] != "wallpaper-a" {
		t.Fatalf("monitor history was not stored in history.json: %#v", mainHistory.Monitors)
	}
	if mainHistory.Current != "" {
		t.Errorf("AddToMonitorHistory() must not update the global current: got %q, want empty", mainHistory.Current)
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

	if err = s.AddToHistoryWithLimit("AAAAA", 50); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}

	history, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if history.Current != "AAAAA" {
		t.Errorf("AddToHistory() Current: got %q, want %q", history.Current, "AAAAA")
	}

	if err = s.AddToHistoryWithLimit("BBBBB", 50); err != nil {
		t.Fatalf("AddToHistory() returned error: %v", err)
	}

	history, err = s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if history.Current != "BBBBB" {
		t.Errorf("AddToHistory() second call Current: got %q, want %q", history.Current, "BBBBB")
	}

	if len(history.Images) != 1 {
		t.Errorf("AddToHistory() Images count: got %d, want 1", len(history.Images))
	}
	if history.Images[0] != "AAAAA" {
		t.Errorf("AddToHistory() history item: got %q, want %q", history.Images[0], "AAAAA")
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

	if err = s.AddToHistoryWithLimit("idNew", 50); err != nil {
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

	if loaded.Images[49] != "id50" {
		t.Errorf("AddToHistory() last item: got %q, want id50", loaded.Images[49])
	}
	if loaded.Current != "idNew" {
		t.Errorf("AddToHistory() current item: got %q, want idNew", loaded.Current)
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

	if err = s.AddToHistoryWithLimit("XXXXX", 50); err != nil {
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

func TestAddToHistoryWithLimit(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	for i := range 12 {
		id := fmt.Sprintf("id%02d", i)
		if err = s.AddToHistoryWithLimit(id, 10); err != nil {
			t.Fatalf("AddToHistoryWithLimit() returned error: %v", err)
		}
	}

	history, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if len(history.Images) != 10 {
		t.Fatalf("history should keep 10 items, got %d", len(history.Images))
	}
	if history.Images[0] != "id01" {
		t.Fatalf("history should keep newest 10, first got %s", history.Images[0])
	}
	if history.Current != "id11" {
		t.Fatalf("current should be latest id11, got %s", history.Current)
	}
}
