package storage

import "time"

// BookmarkData represents the persisted bookmarks file content.
type BookmarkData struct {
	IDs        []string  `json:"ids"`
	LastUpdate time.Time `json:"last_update"`
}

func NewBookmarkData(ids []string) BookmarkData {
	return BookmarkData{
		IDs:        ids,
		LastUpdate: time.Now(),
	}
}
