package storage

import (
	"encoding/json"
	"os"
	"time"

	"github.com/alphonse927/kpixiv/internal/sets"
	"github.com/alphonse927/kpixiv/internal/slices"
)

type History struct {
	Current   string            `json:"current"`
	Images    []string          `json:"images"`
	Monitors  map[string]string `json:"monitors,omitempty"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func NewHistory() *History {
	return &History{
		Images:   []string{},
		Monitors: map[string]string{},
	}
}

func (h *History) Normalize() {
	if h.Images == nil {
		h.Images = []string{}
	}
	if h.Monitors == nil {
		h.Monitors = map[string]string{}
	}
}

func (h *History) SetCurrent(imageID string) {
	if h.Current != "" && h.Current != imageID {
		h.Images = append(h.Images, h.Current)
	}
	h.Current = imageID
}

func (h *History) SetMonitor(screenID, imageID string) {
	if oldID, exists := h.Monitors[screenID]; exists && oldID != "" && oldID != imageID {
		h.Images = append(h.Images, oldID)
	}
	h.Monitors[screenID] = imageID
}

func (h *History) Remove(imageID string) {
	h.Images = slices.Filter(h.Images, func(id string) bool {
		return id != imageID
	})
	if h.Current == imageID {
		h.Current = ""
	}
}

func (h *History) RemoveSet(ids sets.Set[string]) {
	h.Images = slices.Filter(h.Images, func(id string) bool {
		return !ids.Contains(id)
	})
	if ids.Contains(h.Current) {
		h.Current = ""
	}
}

func (h *History) Trim(limit int) {
	limit = max(limit, 1)
	if len(h.Images) <= limit {
		return
	}
	h.Images = h.Images[len(h.Images)-limit:]
}

func (s *Storage) updateHistory(fn func(*History) error) error {
	history, err := s.LoadHistory()
	if err != nil {
		return err
	}
	if err = fn(history); err != nil {
		return err
	}
	return s.SaveHistory(history)
}

func (s *Storage) LoadHistory() (*History, error) {
	path := s.HistoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			h := NewHistory()
			h.UpdatedAt = time.Now()
			return h, nil
		}

		return nil, err
	}

	var history History
	if err = json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	history.Normalize()
	return &history, nil
}

func (s *Storage) SaveHistory(history *History) error {
	history.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.HistoryPath(), data, 0600)
}

func (s *Storage) LoadMonitorHistory() (map[string]string, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return nil, err
	}

	return history.Monitors, nil
}

func (s *Storage) SaveMonitorHistory(monitors map[string]string, historyLimit int) error {
	return s.updateHistory(func(h *History) error {
		for screenID, imageID := range monitors {
			h.SetMonitor(screenID, imageID)
		}
		h.Trim(historyLimit)
		return nil
	})
}

func (s *Storage) AddToMonitorHistory(screenID, imageID string, historyLimit int) error {
	return s.updateHistory(func(h *History) error {
		h.SetMonitor(screenID, imageID)
		h.SetCurrent(imageID)
		h.Trim(historyLimit)
		return nil
	})
}

func (s *Storage) AddToHistoryWithLimit(imageID string, historyLimit int) error {
	return s.updateHistory(func(h *History) error {
		h.SetCurrent(imageID)
		h.Trim(historyLimit)
		return nil
	})
}

func (s *Storage) GetCurrentWallpaper() (string, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return "", err
	}

	return history.Current, nil
}
