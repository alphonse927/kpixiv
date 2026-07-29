package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyImageToDownloadDir(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	sourcePath := filepath.Join(s.RankingDir(), "12345.jpg")
	if err = os.WriteFile(sourcePath, []byte("image-bytes"), 0600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	metadata := map[string]*ImageMeta{
		"12345": {
			ID:    "12345",
			Path:  sourcePath,
			Title: "Title:/With*Invalid?Chars",
		},
	}
	if err = s.SaveMetadata(metadata); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	destPath, err := s.CopyImageToDownloadDir("12345")
	if err != nil {
		t.Fatalf("CopyImageToDownloadDir() returned error: %v", err)
	}

	if filepath.Dir(destPath) != s.DownloadDir() {
		t.Fatalf("CopyImageToDownloadDir() dir: got %q, want %q", filepath.Dir(destPath), s.DownloadDir())
	}

	if filepath.Base(destPath) != "12345 - Title With Invalid Chars.jpg" {
		t.Fatalf("CopyImageToDownloadDir() file: got %q", filepath.Base(destPath))
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}

	if string(data) != "image-bytes" {
		t.Fatalf("copied file contents: got %q", string(data))
	}
}
