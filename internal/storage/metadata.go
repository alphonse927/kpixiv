package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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

func (s *Storage) SaveMetadata(images map[string]*ImageMeta) error {
	data, err := json.MarshalIndent(images, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.MetadataPath(), data, 0600)
}

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
