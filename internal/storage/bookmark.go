package storage

import "time"

// BookmarkData represents the persisted bookmarks file content.
type BookmarkData struct {
	IDs          []string  `json:"ids"`
	LastBookmark time.Time `json:"last_bookmark"`
	LastUpdate   time.Time `json:"last_update"`
}

func NewBookmarkData(ids []string) BookmarkData {
	return BookmarkData{
		IDs:          ids,
		LastBookmark: time.Now(),
		LastUpdate:   time.Now(),
	}
}
