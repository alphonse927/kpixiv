package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alphonse927/kpixiv/internal/sets"
)

// CleanupResult reports what a cleanup pass removed.
type CleanupResult struct {
	Removed    int   // number of image files deleted
	FreedBytes int64 // total bytes freed
}

// CleanupImagesOlderThanDays removes cached wallpapers older than the given
// number of days. A non-positive value removes everything. Bookmarked images
// are never removed. The returned result reports how many files were deleted
// and how much disk space was freed.
func (s *Storage) CleanupImagesOlderThanDays(days int) (CleanupResult, error) {
	images, err := s.LoadMetadata()
	if err != nil {
		return CleanupResult{}, err
	}

	cutoff, removeAll := cleanupCutoff(days)
	removedIDs, removedFiles, metaRemoved, metaFreed, mrErr := s.cleanupMetadata(images, cutoff, removeAll)
	if mrErr != nil {
		return CleanupResult{}, mrErr
	}

	if err = s.SaveMetadata(images); err != nil {
		return CleanupResult{}, err
	}

	fileRemoved, fileFreed, crErr := s.cleanupRankingFiles(cutoff, removeAll, removedFiles)
	if crErr != nil {
		return CleanupResult{}, crErr
	}

	if err = s.cleanupHistory(removedIDs); err != nil {
		return CleanupResult{}, err
	}

	if err = s.cleanupQueue(removedIDs); err != nil {
		return CleanupResult{Removed: metaRemoved + fileRemoved, FreedBytes: metaFreed + fileFreed}, err
	}

	thumbRemoved, thumbFreed, thErr := s.cleanupThumbnails(removedIDs, removeAll)
	if thErr != nil {
		return CleanupResult{Removed: metaRemoved + fileRemoved, FreedBytes: metaFreed + fileFreed}, thErr
	}

	return CleanupResult{Removed: metaRemoved + fileRemoved + thumbRemoved, FreedBytes: metaFreed + fileFreed + thumbFreed}, nil
}

// cleanupThumbnails removes cached thumbnails. When removeAll is true, the
// entire thumbnail directory is cleared; otherwise, only thumbnails whose
// artwork was removed are deleted.
func (s *Storage) cleanupThumbnails(removedIDs sets.Set[string], removeAll bool) (int, int64, error) {
	targets, err := s.cleanupThumbnailTargets(removedIDs, removeAll)
	if err != nil {
		return 0, 0, err
	}

	var removedCount int
	var freedBytes int64
	for _, path := range targets {
		size, removed, err := removeThumbnail(path)
		if err != nil {
			return removedCount, freedBytes, err
		}
		if removed {
			freedBytes += size
			removedCount++
		}
	}

	return removedCount, freedBytes, nil
}

// cleanupThumbnailTargets resolves the list of thumbnail paths to remove,
// either the full thumbnail directory or the thumbnails for removed IDs.
func (s *Storage) cleanupThumbnailTargets(removedIDs sets.Set[string], removeAll bool) ([]string, error) {
	if !removeAll {
		targets := make([]string, 0, len(removedIDs))
		for id := range removedIDs {
			targets = append(targets, s.ThumbnailPath(id))
		}

		return targets, nil
	}

	entries, err := os.ReadDir(s.ThumbnailDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to read thumbnail directory: %w", err)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		targets = append(targets, filepath.Join(s.ThumbnailDir(), entry.Name()))
	}

	return targets, nil
}

// removeThumbnail deletes a single thumbnail file, reporting its size.
func removeThumbnail(path string) (int64, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}

		return 0, false, err
	}

	if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
		return 0, false, rmErr
	}

	return info.Size(), true, nil
}

func (s *Storage) cleanupMetadata(images map[string]*ImageMeta, cutoff time.Time, removeAll bool) (sets.Set[string], sets.Set[string], int, int64, error) {
	removedIDs := sets.New[string]()
	removedFiles := sets.New[string]()
	removedCount := 0
	var freedBytes int64

	for id, meta := range images {
		if meta.Source == "bookmarks" {
			continue
		}

		if !removeAll && !meta.DownloadedAt.Before(cutoff) {
			continue
		}

		if meta.Path != "" {
			if info, rmErr := os.Stat(meta.Path); rmErr == nil {
				freedBytes += info.Size()
			}
			if rmErr := os.Remove(meta.Path); rmErr != nil && !os.IsNotExist(rmErr) {
				return nil, nil, removedCount, freedBytes, fmt.Errorf("failed to remove image file %s: %w", meta.Path, rmErr)
			}
			removedFiles.Add(meta.Path)
		}

		delete(images, id)
		removedIDs.Add(id)
		removedCount++
	}

	return removedIDs, removedFiles, removedCount, freedBytes, nil
}

func (s *Storage) cleanupRankingFiles(cutoff time.Time, removeAll bool, removedFiles sets.Set[string]) (int, int64, error) {
	rankingEntries, readErr := os.ReadDir(s.RankingDir())
	if readErr != nil {
		return 0, 0, fmt.Errorf("failed to read ranking directory: %w", readErr)
	}

	removedCount := 0
	var freedBytes int64
	for _, entry := range rankingEntries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(s.RankingDir(), entry.Name())
		if removedFiles.Contains(path) {
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
			return removedCount, freedBytes, fmt.Errorf("failed to remove ranking image %s: %w", path, rmErr)
		}

		freedBytes += info.Size()
		removedCount++
	}

	return removedCount, freedBytes, nil
}

func (s *Storage) cleanupHistory(removedIDs sets.Set[string]) error {
	return s.updateHistory(func(h *History) error {
		h.RemoveSet(removedIDs)
		return nil
	})
}

func (s *Storage) cleanupQueue(removedIDs sets.Set[string]) error {
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
