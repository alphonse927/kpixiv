package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// jstZone is the timezone used by Pixiv's daily ranking rollover.
var jstZone = time.FixedZone("JST", 9*60*60)

// Activity tracks scheduling state that the daemon and CLI report on: the last
// time background tasks ran and the pagination cursors for ranking fetches and
// bookmark syncs. It is persisted so status survives daemon restarts.
type Activity struct {
	LastFetchAt        time.Time         `json:"last_fetch_at"`
	LastBookmarkSyncAt time.Time         `json:"last_bookmark_sync_at"`
	RankingPages       map[string]int    `json:"ranking_pages,omitempty"`
	RankingDates       map[string]string `json:"ranking_dates,omitempty"`
	LastBookmarkPage   string            `json:"last_bookmark_page,omitempty"`
	BookmarkComplete   bool              `json:"bookmark_complete,omitempty"`
}

func (a *Activity) Normalize() {
	if a.RankingPages == nil {
		a.RankingPages = map[string]int{}
	}

	if a.RankingDates == nil {
		a.RankingDates = map[string]string{}
	}
}

func (s *Storage) ActivityPath() string {
	return filepath.Join(s.stateDir, "activity.json")
}

func (s *Storage) LoadActivity() (*Activity, error) {
	activity, err := s.readActivity()
	if err != nil {
		return nil, err
	}

	activity.Normalize()
	return activity, nil
}

func (s *Storage) readActivity() (*Activity, error) {
	data, err := os.ReadFile(s.ActivityPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Activity{}, nil
		}
		return nil, err
	}

	var activity Activity
	if err = json.Unmarshal(data, &activity); err != nil {
		return nil, err
	}

	activity.Normalize()
	return &activity, nil
}

func (s *Storage) SaveActivity(activity *Activity) error {
	activity.Normalize()
	data, err := json.MarshalIndent(activity, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.ActivityPath(), data, 0600)
}

func (s *Storage) updateActivity(fn func(*Activity) error) error {
	activity, err := s.LoadActivity()
	if err != nil {
		return err
	}
	if err = fn(activity); err != nil {
		return err
	}
	return s.SaveActivity(activity)
}

// RecordFetch stores the timestamp of the last ranking fetch.
func (s *Storage) RecordFetch(t time.Time) error {
	return s.updateActivity(func(activity *Activity) error {
		activity.LastFetchAt = t
		return nil
	})
}

// RecordBookmarkSync stores the timestamp of the last bookmark sync.
func (s *Storage) RecordBookmarkSync(t time.Time) error {
	return s.updateActivity(func(activity *Activity) error {
		activity.LastBookmarkSyncAt = t
		return nil
	})
}

// GetRankingPage returns the persisted ranking page for the given key.
func (s *Storage) GetRankingPage(key string) (int, error) {
	activity, err := s.LoadActivity()
	if err != nil {
		return 1, err
	}

	page, ok := activity.RankingPages[key]
	if !ok || page < 1 {
		return 1, nil
	}

	return page, nil
}

func (s *Storage) SetRankingPage(key string, page int) error {
	if page < 1 {
		page = 1
	}

	return s.updateActivity(func(activity *Activity) error {
		activity.RankingPages[key] = page
		return nil
	})
}

// NextRankingPage returns the ranking page to fetch next for the given key.
// When the ranking has rolled over to a new day, the page is reset to 1 so
// the fresh ranking is crawled from the top instead of continuing from stale
// pages of the previous day.
func (s *Storage) NextRankingPage(key string) (int, error) {
	activity, err := s.LoadActivity()
	if err != nil {
		return 1, err
	}

	today := rankingDate(time.Now())
	if activity.RankingDates[key] != today {
		activity.RankingDates[key] = today
		activity.RankingPages[key] = 1

		if err = s.SaveActivity(activity); err != nil {
			return 1, err
		}

		return 1, nil
	}

	page := activity.RankingPages[key]
	if page < 1 {
		return 1, nil
	}

	return page, nil
}

// GetBookmarkPagination returns the bookmark sync cursor and whether the
// initial full sync has completed.
func (s *Storage) GetBookmarkPagination() (lastPageURL string, complete bool, err error) {
	activity, err := s.LoadActivity()
	if err != nil {
		return "", false, err
	}

	return activity.LastBookmarkPage, activity.BookmarkComplete, nil
}

func (s *Storage) SetBookmarkPagination(lastPageURL string, complete bool) error {
	return s.updateActivity(func(activity *Activity) error {
		activity.LastBookmarkPage = lastPageURL
		activity.BookmarkComplete = complete
		return nil
	})
}

func rankingDate(t time.Time) string {
	return t.In(jstZone).Format("2006-01-02")
}
