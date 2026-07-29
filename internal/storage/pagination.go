package storage

import (
	"encoding/json"
	"os"
)

type PaginationState struct {
	Pages            map[string]int `json:"pages"`
	LastBookmarkPage string         `json:"last_bookmark_page,omitempty"`
	BookmarkComplete bool           `json:"bookmark_complete,omitempty"`
}

func (p *PaginationState) Normalize() {
	if p.Pages == nil {
		p.Pages = map[string]int{}
	}
}

func (s *Storage) updatePagination(fn func(*PaginationState) error) error {
	state, err := s.LoadPaginationState()
	if err != nil {
		return err
	}
	if err = fn(state); err != nil {
		return err
	}
	return s.SavePaginationState(state)
}

func (s *Storage) LoadPaginationState() (*PaginationState, error) {
	path := s.PaginationPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PaginationState{Pages: map[string]int{}}, nil
		}
		return nil, err
	}

	var state PaginationState
	if err = json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	state.Normalize()
	return &state, nil
}

func (s *Storage) SavePaginationState(state *PaginationState) error {
	state.Normalize()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.PaginationPath(), data, 0600)
}

func (s *Storage) GetRankingPage(key string) (int, error) {
	state, err := s.LoadPaginationState()
	if err != nil {
		return 1, err
	}

	page, ok := state.Pages[key]
	if !ok || page < 1 {
		return 1, nil
	}

	return page, nil
}

func (s *Storage) SetRankingPage(key string, page int) error {
	if page < 1 {
		page = 1
	}

	return s.updatePagination(func(state *PaginationState) error {
		state.Pages[key] = page
		return nil
	})
}

func (s *Storage) GetBookmarkPagination() (lastPageURL string, complete bool, err error) {
	state, err := s.LoadPaginationState()
	if err != nil {
		return "", false, err
	}

	return state.LastBookmarkPage, state.BookmarkComplete, nil
}

func (s *Storage) SetBookmarkPagination(lastPageURL string, complete bool) error {
	return s.updatePagination(func(state *PaginationState) error {
		state.LastBookmarkPage = lastPageURL
		state.BookmarkComplete = complete
		return nil
	})
}
