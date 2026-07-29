package storage

import (
	"path/filepath"
	"regexp"
)

var invalidFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func (s *Storage) RankingDir() string {
	return filepath.Join(s.dataDir, "Ranking")
}

func (s *Storage) BookmarksDir() string {
	return filepath.Join(s.dataDir, "Bookmarks")
}

func (s *Storage) ThumbnailDir() string {
	return filepath.Join(s.dataDir, "Thumbnails")
}

func (s *Storage) ThumbnailPath(id string) string {
	return filepath.Join(s.ThumbnailDir(), id+".jpg")
}

func (s *Storage) FavoritesMetadataPath() string {
	return s.MetadataPath()
}

func (s *Storage) MetadataPath() string {
	return filepath.Join(s.stateDir, "metadata.json")
}

func (s *Storage) HistoryPath() string {
	return filepath.Join(s.stateDir, "history.json")
}

func (s *Storage) PaginationPath() string {
	return filepath.Join(s.stateDir, "pagination.json")
}

func (s *Storage) BlacklistPath() string {
	return filepath.Join(s.stateDir, "blacklist.json")
}

func (s *Storage) BookmarkPath() string {
	return filepath.Join(s.stateDir, "bookmarks.json")
}
