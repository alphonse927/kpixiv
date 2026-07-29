package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *Storage) CleanupImagesOlderThanDays(days int) (int, error) {
	images, err := s.LoadMetadata()
	if err != nil {
		return 0, err
	}

	cutoff, removeAll := cleanupCutoff(days)
	removedIDs, removedFiles, removedFromMetadata, mrErr := s.cleanupMetadata(images, cutoff, removeAll)
	if mrErr != nil {
		return 0, mrErr
	}

	if err = s.SaveMetadata(images); err != nil {
		return 0, err
	}

	removedFromRanking, crErr := s.cleanupRankingFiles(cutoff, removeAll, removedFiles)
	if crErr != nil {
		return 0, crErr
	}

	if err = s.cleanupHistory(removedIDs); err != nil {
		return 0, err
	}

	if err = s.cleanupQueue(removedIDs); err != nil {
		return removedFromMetadata + removedFromRanking, err
	}

	return removedFromMetadata + removedFromRanking, nil
}

func (s *Storage) cleanupMetadata(images map[string]*ImageMeta, cutoff time.Time, removeAll bool) (map[string]struct{}, map[string]struct{}, int, error) {
	removedIDs := make(map[string]struct{})
	removedFiles := make(map[string]struct{})
	removedCount := 0

	for id, meta := range images {
		if meta.Source == "bookmarks" {
			continue
		}

		if !removeAll && !meta.DownloadedAt.Before(cutoff) {
			continue
		}

		if meta.Path != "" {
			if rmErr := os.Remove(meta.Path); rmErr != nil && !os.IsNotExist(rmErr) {
				return nil, nil, removedCount, fmt.Errorf("failed to remove image file %s: %w", meta.Path, rmErr)
			}
			removedFiles[meta.Path] = struct{}{}
		}

		delete(images, id)
		removedIDs[id] = struct{}{}
		removedCount++
	}

	return removedIDs, removedFiles, removedCount, nil
}

func (s *Storage) cleanupRankingFiles(cutoff time.Time, removeAll bool, removedFiles map[string]struct{}) (int, error) {
	rankingEntries, readErr := os.ReadDir(s.RankingDir())
	if readErr != nil {
		return 0, fmt.Errorf("failed to read ranking directory: %w", readErr)
	}

	removedCount := 0
	for _, entry := range rankingEntries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(s.RankingDir(), entry.Name())
		if _, alreadyRemoved := removedFiles[path]; alreadyRemoved {
			continue
		}

		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}

		if !removeAll && !info.ModTime().Before(cutoff) {
			continue
		}

		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return removedCount, fmt.Errorf("failed to remove ranking image %s: %w", path, rmErr)
		}

		removedCount++
	}

	return removedCount, nil
}

func (s *Storage) cleanupHistory(removedIDs map[string]struct{}) error {
	return s.updateHistory(func(h *History) error {
		h.RemoveSet(removedIDs)
		return nil
	})
}

func (s *Storage) cleanupQueue(removedIDs map[string]struct{}) error {
	q := NewQueue(s.stateDir)
	if err := q.Load(); err != nil {
		return err
	}

	if q.IsEmpty() {
		return nil
	}

	for id := range removedIDs {
		if err := q.Remove(id); err != nil {
			return err
		}
	}

	return nil
}

func cleanupCutoff(days int) (time.Time, bool) {
	removeAll := days <= 0
	if removeAll {
		return time.Now(), true
	}

	return time.Now().Add(-time.Duration(days) * 24 * time.Hour), false
}
