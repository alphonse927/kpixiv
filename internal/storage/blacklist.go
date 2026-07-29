package storage

import (
	"encoding/json"
	"os"
	"time"

	"github.com/alphonse927/kpixiv/internal/slices"
)

type Blacklist struct {
	IDs       []string  `json:"ids"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b *Blacklist) Normalize() {
	if b.IDs == nil {
		b.IDs = []string{}
	}
}

func (b *Blacklist) Add(id string) {
	b.IDs = slices.Unique(append(b.IDs, id))
	b.UpdatedAt = time.Now()
}

func (b *Blacklist) Contains(id string) bool {
	for _, existing := range b.IDs {
		if existing == id {
			return true
		}
	}
	return false
}

func (s *Storage) LoadBlacklist() (*Blacklist, error) {
	path := s.BlacklistPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Blacklist{IDs: []string{}, UpdatedAt: time.Now()}, nil
		}
		return nil, err
	}

	var blacklist Blacklist
	if err = json.Unmarshal(data, &blacklist); err != nil {
		return nil, err
	}

	blacklist.Normalize()
	return &blacklist, nil
}

func (s *Storage) SaveBlacklist(blacklist *Blacklist) error {
	blacklist.Normalize()
	blacklist.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(blacklist, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.BlacklistPath(), data, 0600)
}

func (s *Storage) LoadBlacklistSet() (map[string]struct{}, error) {
	blacklist, err := s.LoadBlacklist()
	if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{}, len(blacklist.IDs))
	for _, id := range blacklist.IDs {
		ids[id] = struct{}{}
	}

	return ids, nil
}

func (s *Storage) ExcludeWallpaper(imageID string) error {
	blacklist, err := s.LoadBlacklist()
	if err != nil {
		return err
	}

	blacklist.Add(imageID)
	if err = s.SaveBlacklist(blacklist); err != nil {
		return err
	}

	return s.updateHistory(func(h *History) error {
		h.Remove(imageID)
		return nil
	})
}
