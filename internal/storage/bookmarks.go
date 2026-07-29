package storage

import (
	"encoding/json"
	"os"
	"time"

	"github.com/alphonse927/kpixiv/internal/slices"
)

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

func (b *BookmarkData) Add(id string) {
	if b.Contains(id) {
		return
	}
	b.IDs = append(b.IDs, id)
	b.LastUpdate = time.Now()
}

func (b *BookmarkData) AddMany(ids []string) {
	all := make([]string, 0, len(b.IDs)+len(ids))
	all = append(all, b.IDs...)
	all = append(all, ids...)
	b.IDs = slices.Unique(all)
	b.LastUpdate = time.Now()
}

func (b *BookmarkData) Contains(id string) bool {
	for _, existing := range b.IDs {
		if existing == id {
			return true
		}
	}
	return false
}

func (b *BookmarkData) AsSet() map[string]struct{} {
	set := make(map[string]struct{}, len(b.IDs))
	for _, id := range b.IDs {
		set[id] = struct{}{}
	}
	return set
}

func (s *Storage) loadBookmarkData() (*BookmarkData, error) {
	path := s.BookmarkPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			bd := NewBookmarkData([]string{})
			return &bd, nil
		}
		return nil, err
	}

	var bd BookmarkData
	if err = json.Unmarshal(data, &bd); err == nil && bd.IDs != nil {
		return &bd, nil
	}

	var ids []string
	if err = json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}

	bd = NewBookmarkData(ids)
	return &bd, nil
}

func (s *Storage) updateBookmarks(fn func(*BookmarkData) error) error {
	bd, err := s.loadBookmarkData()
	if err != nil {
		return err
	}
	if err = fn(bd); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.BookmarkPath(), data, 0600)
}

func (s *Storage) LoadBookmarks() (map[string]struct{}, error) {
	bd, err := s.loadBookmarkData()
	if err != nil {
		return nil, err
	}

	return bd.AsSet(), nil
}

func (s *Storage) SaveBookmarks(ids []string) error {
	if ids == nil {
		ids = []string{}
	}

	bd := NewBookmarkData(ids)
	data, err := json.MarshalIndent(bd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.BookmarkPath(), data, 0600)
}

func (s *Storage) IsArtworkBookmarked(id string) (bool, error) {
	bd, err := s.loadBookmarkData()
	if err != nil {
		return false, err
	}

	return bd.Contains(id), nil
}

func (s *Storage) AddBookmark(id string) error {
	return s.updateBookmarks(func(bd *BookmarkData) error {
		bd.Add(id)
		return nil
	})
}

func (s *Storage) AddBookmarks(ids []string) error {
	return s.updateBookmarks(func(bd *BookmarkData) error {
		bd.AddMany(ids)
		return nil
	})
}
