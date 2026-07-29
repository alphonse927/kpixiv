package storage

import (
	"os"
	"path/filepath"
)

type Storage struct {
	dataDir      string
	downloadDir  string
	downloadPath string
	homeDir      string
	stateDir     string
}

func New(homeDir, downloadPath string) (*Storage, error) {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir() //nolint:errcheck
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

func (s *Storage) DataDir() string {
	return s.dataDir
}

func (s *Storage) StateDir() string {
	return s.stateDir
}

func (s *Storage) DownloadDir() string {
	return s.downloadDir
}

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
