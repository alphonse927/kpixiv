package storage

import (
	"testing"

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
