package storage

import (
	"os"
	"testing"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
)

func TestRecordAndLoadActivity(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	activity, err := s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}
	if !activity.LastFetchAt.IsZero() || !activity.LastBookmarkSyncAt.IsZero() {
		t.Errorf("fresh activity should be zero-valued, got %+v", activity)
	}

	fetchTime := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	if err = s.RecordFetch(fetchTime); err != nil {
		t.Fatalf("RecordFetch() returned error: %v", err)
	}

	syncTime := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	if err = s.RecordBookmarkSync(syncTime); err != nil {
		t.Fatalf("RecordBookmarkSync() returned error: %v", err)
	}

	activity, err = s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}
	if !activity.LastFetchAt.Equal(fetchTime) {
		t.Errorf("LastFetchAt: got %v, want %v", activity.LastFetchAt, fetchTime)
	}
	if !activity.LastBookmarkSyncAt.Equal(syncTime) {
		t.Errorf("LastBookmarkSyncAt: got %v, want %v", activity.LastBookmarkSyncAt, syncTime)
	}
}

func TestSaveAndLoadActivity(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	original := &Activity{
		RankingPages: map[string]int{
			"daily_r18": 3,
			"weekly":    5,
		},
		RankingDates: map[string]string{
			"daily_r18": "2026-08-07",
		},
	}

	if err = s.SaveActivity(original); err != nil {
		t.Fatalf("SaveActivity() returned error: %v", err)
	}

	loaded, err := s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}

	if loaded.RankingPages["daily_r18"] != 3 {
		t.Errorf("daily_r18: got %d, want 3", loaded.RankingPages["daily_r18"])
	}
	if loaded.RankingPages["weekly"] != 5 {
		t.Errorf("weekly: got %d, want 5", loaded.RankingPages["weekly"])
	}
	if loaded.RankingDates["daily_r18"] != "2026-08-07" {
		t.Errorf("daily_r18 date: got %q, want %q", loaded.RankingDates["daily_r18"], "2026-08-07")
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

	activity, err := s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}
	if got := activity.RankingDates[key]; got != rankingDate(time.Now()) {
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
	original := &Activity{
		RankingPages: map[string]int{key: 5},
		RankingDates: map[string]string{key: today},
	}
	if err = s.SaveActivity(original); err != nil {
		t.Fatalf("SaveActivity() returned error: %v", err)
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
	original := &Activity{
		RankingPages: map[string]int{key: 9},
		RankingDates: map[string]string{key: "2000-01-01"},
	}
	if err = s.SaveActivity(original); err != nil {
		t.Fatalf("SaveActivity() returned error: %v", err)
	}

	page, err := s.NextRankingPage(key)
	if err != nil {
		t.Fatalf("NextRankingPage() returned error: %v", err)
	}
	if page != 1 {
		t.Errorf("NextRankingPage() after rollover: got %d, want 1", page)
	}

	activity, err := s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}
	if page := activity.RankingPages[key]; page != 1 {
		t.Errorf("RankingPages[%s] after rollover: got %d, want 1", key, page)
	}
	if got := activity.RankingDates[key]; got != rankingDate(time.Now()) {
		t.Errorf("ranking date after rollover: got %q, want %q", got, rankingDate(time.Now()))
	}
}

func TestBookmarkPagination(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	lastURL, complete, err := s.GetBookmarkPagination()
	if err != nil {
		t.Fatalf("GetBookmarkPagination() returned error: %v", err)
	}
	if lastURL != "" || complete {
		t.Errorf("fresh bookmark pagination: got %q/%v, want empty/false", lastURL, complete)
	}

	next := "https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=1&next=abc"
	if err = s.SetBookmarkPagination(next, false); err != nil {
		t.Fatalf("SetBookmarkPagination() returned error: %v", err)
	}

	lastURL, complete, err = s.GetBookmarkPagination()
	if err != nil {
		t.Fatalf("GetBookmarkPagination() returned error: %v", err)
	}
	if lastURL != next || complete {
		t.Errorf("GetBookmarkPagination(): got %q/%v, want %q/false", lastURL, complete, next)
	}

	if err = s.SetBookmarkPagination("", true); err != nil {
		t.Fatalf("SetBookmarkPagination(complete) returned error: %v", err)
	}
	lastURL, complete, err = s.GetBookmarkPagination()
	if err != nil {
		t.Fatalf("GetBookmarkPagination() returned error: %v", err)
	}
	if lastURL != "" || !complete {
		t.Errorf("GetBookmarkPagination() after complete: got %q/%v, want empty/true", lastURL, complete)
	}
}

func TestLoadActivityMigratesLegacyPagination(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	legacy := `{
  "pages": { "daily:false": 3 },
  "ranking_dates": { "daily:false": "2026-08-07" },
  "last_bookmark_page": "https://app-api.pixiv.net/v1/user/bookmarks/illust?next=x",
  "bookmark_complete": true
}`
	if err = os.WriteFile(s.legacyPaginationPath(), []byte(legacy), 0600); err != nil {
		t.Fatalf("writing legacy pagination: %v", err)
	}

	activity, err := s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}
	if got := activity.RankingPages["daily:false"]; got != 3 {
		t.Errorf("migrated ranking page: got %d, want 3", got)
	}
	if got := activity.RankingDates["daily:false"]; got != "2026-08-07" {
		t.Errorf("migrated ranking date: got %q, want %q", got, "2026-08-07")
	}
	if got := activity.LastBookmarkPage; got == "" || !activity.BookmarkComplete {
		t.Errorf("migrated bookmark state: got %q/%v", got, activity.BookmarkComplete)
	}

	if _, err = os.Stat(s.legacyPaginationPath()); !os.IsNotExist(err) {
		t.Errorf("legacy pagination.json should be removed after migration, stat err=%v", err)
	}

	page, err := s.GetRankingPage("daily:false")
	if err != nil {
		t.Fatalf("GetRankingPage() after migration returned error: %v", err)
	}
	if page != 3 {
		t.Errorf("GetRankingPage() after migration: got %d, want 3", page)
	}
}

func TestLoadActivityIgnoresMissingLegacyPagination(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, tmp)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	activity, err := s.LoadActivity()
	if err != nil {
		t.Fatalf("LoadActivity() returned error: %v", err)
	}
	if len(activity.RankingPages) != 0 || len(activity.RankingDates) != 0 {
		t.Errorf("fresh activity should have no ranking state, got %+v", activity)
	}
}
