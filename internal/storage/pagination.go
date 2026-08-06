package storage

import (
	"encoding/json"
	"os"
	"time"
)

type PaginationState struct {
	Pages            map[string]int    `json:"pages"`
	RankingDates     map[string]string `json:"ranking_dates,omitempty"`
	LastBookmarkPage string            `json:"last_bookmark_page,omitempty"`
	BookmarkComplete bool              `json:"bookmark_complete,omitempty"`
}

func (p *PaginationState) Normalize() {
	if p.Pages == nil {
		p.Pages = map[string]int{}
	}
	if p.RankingDates == nil {
		p.RankingDates = map[string]string{}
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
			return &PaginationState{
				Pages:        map[string]int{},
				RankingDates: map[string]string{},
			}, nil
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

// jstZone is the timezone used by Pixiv's daily ranking rollover.
var jstZone = time.FixedZone("JST", 9*60*60)

func rankingDate(t time.Time) string {
	return t.In(jstZone).Format("2006-01-02")
}

// NextRankingPage returns the ranking page to fetch next for the given key.
// When the ranking has rolled over to a new day, the page is reset to 1 so
// the fresh ranking is crawled from the top instead of continuing from stale
// pages of the previous day.
func (s *Storage) NextRankingPage(key string) (int, error) {
	state, err := s.LoadPaginationState()
	if err != nil {
		return 1, err
	}

	today := rankingDate(time.Now())
	if state.RankingDates[key] != today {
		state.RankingDates[key] = today
		state.Pages[key] = 1
		if err = s.SavePaginationState(state); err != nil {
			return 1, err
		}
		return 1, nil
	}

	page := state.Pages[key]
	if page < 1 {
		return 1, nil
	}

	return page, nil
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
