package storage

import (
	"os"
	"time"
)

// CacheStats summarizes the downloaded wallpaper cache without walking the
// whole filesystem. Oldest/Newest come from recorded download times, and
// Size is computed from each known metadata file, which is cheap for the
// typical cache sizes this application manages.
type CacheStats struct {
	Count  int
	Size   int64
	Oldest time.Time
	Newest time.Time
}

// CacheStats computes statistics from the metadata store. It is safe to call
// on every status refresh.
func (s *Storage) CacheStats() (CacheStats, error) {
	images, err := s.LoadMetadata()
	if err != nil {
		return CacheStats{}, err
	}

	var stats CacheStats
	for _, meta := range images {
		if meta == nil {
			continue
		}
		stats.Count++

		if info, statErr := os.Stat(meta.Path); statErr == nil {
			stats.Size += info.Size()
		}

		if meta.DownloadedAt.IsZero() {
			continue
		}
		if stats.Oldest.IsZero() || meta.DownloadedAt.Before(stats.Oldest) {
			stats.Oldest = meta.DownloadedAt
		}
		if meta.DownloadedAt.After(stats.Newest) {
			stats.Newest = meta.DownloadedAt
		}
	}

	return stats, nil
}
