package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (s *Storage) CopyImageToDownloadDir(id string) (string, error) {
	sourcePath, ok := s.GetImagePath(id)
	if !ok {
		return "", fmt.Errorf("artwork %s not found on disk", id)
	}

	meta, _ := s.lookupImageMeta(id)
	filename := s.downloadFilename(id, sourcePath, meta)
	destPath := filepath.Join(s.downloadDir, filename)

	if filepath.Clean(sourcePath) == filepath.Clean(destPath) {
		return destPath, nil
	}

	if err := copyFileAtomic(sourcePath, destPath); err != nil {
		return "", err
	}

	return destPath, nil
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

func resolveDownloadDir(homeDir, downloadPath string) string {
	trimmed := strings.TrimSpace(downloadPath)
	if trimmed == "" {
		return filepath.Join(homeDir, "Pictures", "KPixiv")
	}

	if trimmed == "~" {
		return homeDir
	}

	if after, ok := strings.CutPrefix(trimmed, "~/"); ok {
		return filepath.Join(homeDir, after)
	}

	return trimmed
}

func copyFileAtomic(sourcePath, destPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source artwork: %w", err)
	}
	defer source.Close() //nolint:errcheck

	if err = os.MkdirAll(filepath.Dir(destPath), 0750); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), ".copy-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary download file: %w", err)
	}

	tmpPath := tmpFile.Name()
	defer tmpFile.Close()    //nolint:errcheck
	defer os.Remove(tmpPath) //nolint:errcheck

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
