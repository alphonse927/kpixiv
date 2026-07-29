package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hasImage(s *Storage, id string) (bool, error) {
	images, err := s.LoadMetadata()
	if err != nil {
		return false, err
	}

	_, exists := images[id]
	return exists, nil
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

func TestSaveMetadataAddsImage(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	imgPath := filepath.Join(tmp, "some-image.jpg")
	if err = os.WriteFile(imgPath, []byte("fake-image-data"), 0600); err != nil {
		t.Fatalf("failed to create test image: %v", err)
	}

	meta := &ImageMeta{
		ID:    "11111",
		Path:  imgPath,
		Width: 1920,
	}

	images := map[string]*ImageMeta{
		meta.ID: meta,
	}

	if err = s.SaveMetadata(images); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	has, err := hasImage(s, "11111")
	if err != nil {
		t.Fatalf("hasImage() returned error: %v", err)
	}
	if !has {
		t.Error("hasImage() after AddImage: got false, want true")
	}

	path, ok := s.GetImagePath("11111")
	if !ok {
		t.Error("GetImagePath() after AddImage: got false, want true")
	}
	if path != imgPath {
		t.Errorf("GetImagePath() path: got %q, want %q", path, imgPath)
	}

	has, err = hasImage(s, "99999")
	if err != nil {
		t.Fatalf("hasImage() returned error: %v", err)
	}
	if has {
		t.Error("hasImage() for non-existent ID: got true, want false")
	}
}
