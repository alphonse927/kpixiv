package storage

import (
	"testing"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
)

func TestLoadPaginationStateEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	state, err := s.LoadPaginationState()
	if err != nil {
		t.Fatalf("LoadPaginationState() returned error: %v", err)
	}

	if state == nil {
		t.Fatal("LoadPaginationState() returned nil")
	}

	if state.Pages == nil {
		t.Error("Pages should not be nil for empty state")
	}
}

func TestSaveAndLoadPaginationState(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	original := &PaginationState{
		Pages: map[string]int{
			"daily_r18": 3,
			"weekly":    5,
		},
	}

	if err = s.SavePaginationState(original); err != nil {
		t.Fatalf("SavePaginationState() returned error: %v", err)
	}

	loaded, err := s.LoadPaginationState()
	if err != nil {
		t.Fatalf("LoadPaginationState() returned error: %v", err)
	}

	if loaded.Pages["daily_r18"] != 3 {
		t.Errorf("daily_r18: got %d, want 3", loaded.Pages["daily_r18"])
	}

	if loaded.Pages["weekly"] != 5 {
		t.Errorf("weekly: got %d, want 5", loaded.Pages["weekly"])
	}
}

func TestGetRankingPage(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	page, err := s.GetRankingPage(config.RankingDailyMode.String())
	if err != nil {
		t.Fatalf("GetRankingPage() returned error: %v", err)
	}

	if page != 1 {
		t.Errorf("GetRankingPage() for unknown key: got %d, want 1", page)
	}

	if err = s.SetRankingPage(config.RankingDailyMode.String(), 4); err != nil {
		t.Fatalf("SetRankingPage() returned error: %v", err)
	}

	page, err = s.GetRankingPage(config.RankingDailyMode.String())
	if err != nil {
		t.Fatalf("GetRankingPage() returned error: %v", err)
	}

	if page != 4 {
		t.Errorf("GetRankingPage() after SetRankingPage: got %d, want 4", page)
	}
}

func TestSetRankingPageClamping(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if err = s.SetRankingPage(config.RankingDailyMode.String(), 0); err != nil {
		t.Fatalf("SetRankingPage(0) returned error: %v", err)
	}

	page, err := s.GetRankingPage(config.RankingDailyMode.String())
	if err != nil {
		t.Fatalf("GetRankingPage() returned error: %v", err)
	}
	if page != 1 {
		t.Errorf("SetRankingPage(0) should clamp to 1: got %d", page)
	}
}

func TestNextRankingPageFreshState(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	key := config.RankingDailyMode.String() + ":false"
	page, err := s.NextRankingPage(key)
	if err != nil {
		t.Fatalf("NextRankingPage() returned error: %v", err)
	}
	if page != 1 {
		t.Errorf("NextRankingPage() for unknown key: got %d, want 1", page)
	}

	state, err := s.LoadPaginationState()
	if err != nil {
		t.Fatalf("LoadPaginationState() returned error: %v", err)
	}
	if got := state.RankingDates[key]; got != rankingDate(time.Now()) {
		t.Errorf("ranking date recorded: got %q, want %q", got, rankingDate(time.Now()))
	}
}

func TestNextRankingPageSameDay(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	key := config.RankingDailyMode.String() + ":false"
	today := rankingDate(time.Now())
	original := &PaginationState{
		Pages:        map[string]int{key: 5},
		RankingDates: map[string]string{key: today},
	}
	if err = s.SavePaginationState(original); err != nil {
		t.Fatalf("SavePaginationState() returned error: %v", err)
	}

	page, err := s.NextRankingPage(key)
	if err != nil {
		t.Fatalf("NextRankingPage() returned error: %v", err)
	}
	if page != 5 {
		t.Errorf("NextRankingPage() same day: got %d, want 5", page)
	}
}

func TestNextRankingPageRollsOverOnNewDay(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	key := config.RankingDailyMode.String() + ":false"
	original := &PaginationState{
		Pages:        map[string]int{key: 9},
		RankingDates: map[string]string{key: "2000-01-01"},
	}
	if err = s.SavePaginationState(original); err != nil {
		t.Fatalf("SavePaginationState() returned error: %v", err)
	}

	page, err := s.NextRankingPage(key)
	if err != nil {
		t.Fatalf("NextRankingPage() returned error: %v", err)
	}
	if page != 1 {
		t.Errorf("NextRankingPage() after rollover: got %d, want 1", page)
	}

	state, err := s.LoadPaginationState()
	if err != nil {
		t.Fatalf("LoadPaginationState() returned error: %v", err)
	}
	if page := state.Pages[key]; page != 1 {
		t.Errorf("Pages[%s] after rollover: got %d, want 1", key, page)
	}
	if got := state.RankingDates[key]; got != rankingDate(time.Now()) {
		t.Errorf("ranking date after rollover: got %q, want %q", got, rankingDate(time.Now()))
	}
}
