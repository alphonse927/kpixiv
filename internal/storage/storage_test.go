package storage

import (
	"os"
	"path/filepath"
	"testing"
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

func TestNewExpandsTildeDownloadPath(t *testing.T) {
	tmp := t.TempDir()
	tildePath := "~/Pictures/KPixiv"
	expected := filepath.Join(tmp, "Pictures", "KPixiv")

	s, err := New(tmp, tildePath)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if s.DownloadDir() != expected {
		t.Fatalf("DownloadDir: got %s, want %s", s.DownloadDir(), expected)
	}

	if _, err = os.Stat(expected); err != nil {
		t.Fatalf("expanded download directory not created: %v", err)
	}

	if _, err = os.Stat(filepath.Join(tmp, "~")); !os.IsNotExist(err) {
		t.Fatalf("tilde-literal directory should not exist, stat err: %v", err)
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
		t.Errorf("GetNextWallpaper() with empty queue: got %q, want empty", next)
	}

	q := NewQueue(s.stateDir)
	if err = q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if err = q.AppendRandom([]string{"AAAAA"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	next, err = s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next != "AAAAA" {
		t.Errorf("GetNextWallpaper() with one item: got %q, want %q", next, "AAAAA")
	}

	peek, ok := q.Peek()
	if !ok || peek != "AAAAA" {
		t.Errorf("GetNextWallpaper() modified queue: Peek() = %q, %v, want %q, true", peek, ok, "AAAAA")
	}
}

func TestGetNextWallpaperMultipleItems(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	q := NewQueue(s.stateDir)
	if err = q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if err = q.AppendRandom([]string{"AAAAA", "BBBBB", "CCCCC"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	next, err := s.GetNextWallpaper()
	if err != nil {
		t.Fatalf("GetNextWallpaper() returned error: %v", err)
	}
	if next == "" {
		t.Fatal("GetNextWallpaper() with items: got empty, want non-empty")
	}

	peek, _ := q.Peek()
	if next != peek {
		t.Errorf("GetNextWallpaper() returned %q but queue Peek() shows %q", next, peek)
	}
}

func TestGetNextWallpaperEmptyQueue(t *testing.T) {
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
		t.Errorf("GetNextWallpaper() with empty queue: got %q, want empty", next)
	}
}
