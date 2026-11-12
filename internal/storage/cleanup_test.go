package storage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestCleanupImagesOlderThanDays(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	oldPath := filepath.Join(s.RankingDir(), "old.jpg")
	newPath := filepath.Join(s.RankingDir(), "new.jpg")
	if err = os.WriteFile(oldPath, []byte("old"), 0600); err != nil {
		t.Fatalf("failed writing old image: %v", err)
	}
	if err = os.WriteFile(newPath, []byte("new"), 0600); err != nil {
		t.Fatalf("failed writing new image: %v", err)
	}

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err = os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed setting old file times: %v", err)
	}

	meta := map[string]*ImageMeta{
		"old": {ID: "old", Path: oldPath, DownloadedAt: oldTime},
		"new": {ID: "new", Path: newPath, DownloadedAt: time.Now()},
	}
	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	// "old" is no longer in use (the current wallpaper is "new"), so it is
	// eligible for cleanup. Wallpapers in use are covered by
	// TestCleanupKeepsWallpapersInUse.
	history := &History{Current: "new", Images: []string{"old"}, UpdatedAt: time.Now()}
	if err = s.SaveHistory(history); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	result, err := s.CleanupImagesOlderThanDays(7)
	if err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}

	if result.Removed < 1 {
		t.Fatalf("expected at least one removed image, got %d", result.Removed)
	}

	if result.FreedBytes < 3 {
		t.Fatalf("expected freed bytes for the removed image, got %d", result.FreedBytes)
	}

	if _, statErr := os.Stat(oldPath); !os.IsNotExist(statErr) {
		t.Fatalf("old file should be removed, stat error: %v", statErr)
	}

	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Fatalf("new file should remain, got stat error: %v", statErr)
	}

	images, err := s.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() returned error: %v", err)
	}

	if _, exists := images["old"]; exists {
		t.Fatalf("old metadata entry should be removed")
	}

	if _, exists := images["new"]; !exists {
		t.Fatalf("new metadata entry should remain")
	}
}

func TestCleanupImagesOlderThanDaysResetRemovesAll(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	path := filepath.Join(s.RankingDir(), "all.jpg")
	if err = os.WriteFile(path, []byte("all"), 0600); err != nil {
		t.Fatalf("failed writing image: %v", err)
	}

	meta := map[string]*ImageMeta{
		"all": {ID: "all", Path: path, DownloadedAt: time.Now()},
	}

	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	result, err := s.CleanupImagesOlderThanDays(0)
	if err != nil {
		t.Fatalf("CleanupImagesOlderThanDays(0) returned error: %v", err)
	}

	if result.Removed < 1 {
		t.Fatalf("expected all images to be removed, got %d", result.Removed)
	}

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("image should be removed by reset cleanup, stat error: %v", statErr)
	}
}

func TestCleanupKeepsWallpapersInUse(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	write := func(name string) string {
		p := filepath.Join(s.RankingDir(), name)
		if err = os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatalf("failed writing image: %v", err)
		}
		return p
	}

	current, monitor, other := write("cur.jpg"), write("mon.jpg"), write("old.jpg")
	old := time.Now().Add(-30 * 24 * time.Hour)
	meta := map[string]*ImageMeta{
		"cur": {ID: "cur", Path: current, DownloadedAt: old},
		"mon": {ID: "mon", Path: monitor, DownloadedAt: old},
		"old": {ID: "old", Path: other, DownloadedAt: old},
	}
	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}
	if err = s.AddToHistoryWithLimit("cur", 10); err != nil {
		t.Fatalf("AddToHistoryWithLimit() returned error: %v", err)
	}
	if err = s.AddToMonitorHistory("DP-1", "mon", 10); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}

	for _, days := range []int{7, 0} {
		if _, err = s.CleanupImagesOlderThanDays(days); err != nil {
			t.Fatalf("CleanupImagesOlderThanDays(%d) returned error: %v", days, err)
		}
		for _, p := range []string{current, monitor} {
			if _, statErr := os.Stat(p); statErr != nil {
				t.Fatalf("days=%d: in-use wallpaper %s was removed: %v", days, p, statErr)
			}
		}
	}
	if _, statErr := os.Stat(other); !os.IsNotExist(statErr) {
		t.Fatalf("unused wallpaper should be removed, stat error: %v", statErr)
	}
}

// cleanupFixture holds a storage with one image old enough to be cleaned up
// ("old"), one recent image that is kept ("new"), and the matching thumbnails.
type cleanupFixture struct {
	s       *Storage
	oldPath string
	oldTime time.Time
}

func newCleanupFixture(t *testing.T) *cleanupFixture {
	t.Helper()

	tmp := t.TempDir()
	s, err := New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	f := &cleanupFixture{s: s, oldTime: time.Now().Add(-10 * 24 * time.Hour)}
	f.oldPath = f.writeRanking(t, "old", f.oldTime)
	newPath := f.writeRanking(t, "new", time.Now())

	meta := map[string]*ImageMeta{
		"old": {ID: "old", Path: f.oldPath, DownloadedAt: f.oldTime},
		"new": {ID: "new", Path: newPath, DownloadedAt: time.Now()},
	}
	if err = s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}
	if err = s.AddToHistoryWithLimit("new", 10); err != nil {
		t.Fatalf("AddToHistoryWithLimit() returned error: %v", err)
	}

	return f
}

func (f *cleanupFixture) writeRanking(t *testing.T, id string, modTime time.Time) string {
	t.Helper()

	path := filepath.Join(f.s.RankingDir(), id+".jpg")
	if err := os.WriteFile(path, []byte(id), 0600); err != nil {
		t.Fatalf("failed writing image %s: %v", id, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("failed setting times for %s: %v", id, err)
	}

	return path
}

func (f *cleanupFixture) writeThumbnail(t *testing.T, id string) string {
	t.Helper()

	path := f.s.ThumbnailPath(id)
	if err := os.WriteFile(path, []byte("thumb"), 0600); err != nil {
		t.Fatalf("failed writing thumbnail %s: %v", id, err)
	}

	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

func TestCleanupRemovesImagesFromGlobalAndMonitorQueues(t *testing.T) {
	f := newCleanupFixture(t)
	s := f.s

	if err := NewQueue(s.StateDir()).AppendRandom([]string{"old", "new"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}
	for _, screen := range []string{"DP-1", "HDMI-1"} {
		if err := NewMonitorQueue(s.StateDir(), screen).AppendRandom([]string{"old", "new"}); err != nil {
			t.Fatalf("AppendRandom() returned error: %v", err)
		}
	}

	if _, err := s.CleanupImagesOlderThanDays(7); err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}

	queues := map[string]*Queue{
		"global": NewQueue(s.StateDir()),
		"DP-1":   NewMonitorQueue(s.StateDir(), "DP-1"),
		"HDMI-1": NewMonitorQueue(s.StateDir(), "HDMI-1"),
	}
	for name, q := range queues {
		if err := q.Load(); err != nil {
			t.Fatalf("Load(%s) returned error: %v", name, err)
		}
		if got := q.GetAll(); !slices.Equal(got, []string{"new"}) {
			t.Errorf("%s queue after cleanup: got %v, want [new]", name, got)
		}
	}
}

func TestCleanupRemovesThumbnailsThatNoLongerHaveAnImage(t *testing.T) {
	f := newCleanupFixture(t)
	s := f.s

	oldThumb := f.writeThumbnail(t, "old")         // image removed by this cleanup
	newThumb := f.writeThumbnail(t, "new")         // image still known
	excludedThumb := f.writeThumbnail(t, "banned") // image excluded earlier: no metadata entry
	ghostThumb := f.writeThumbnail(t, "ghost")     // file without metadata, removed by cleanup
	f.writeRanking(t, "ghost", f.oldTime)
	otherFile := filepath.Join(s.ThumbnailDir(), "notes.txt")
	if err := os.WriteFile(otherFile, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	if _, err := s.CleanupImagesOlderThanDays(7); err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}

	for name, path := range map[string]string{"old": oldThumb, "banned": excludedThumb, "ghost": ghostThumb} {
		if exists(path) {
			t.Errorf("thumbnail of %q should have been removed", name)
		}
	}
	if !exists(newThumb) {
		t.Error("thumbnail of a known image must be kept")
	}
	if !exists(otherFile) {
		t.Error("files that are not thumbnails must be left alone")
	}
}

func TestCleanupDropsUntrackedFilesFromHistoryAndQueues(t *testing.T) {
	f := newCleanupFixture(t)
	s := f.s

	// "ghost" has a file but no metadata entry; cleanup deletes the file, so
	// its ID must leave the history and every queue as well.
	f.writeRanking(t, "ghost", f.oldTime)
	if err := s.updateHistory(func(h *History) error {
		h.Images = append(h.Images, "ghost")
		return nil
	}); err != nil {
		t.Fatalf("updateHistory() returned error: %v", err)
	}
	if err := NewQueue(s.StateDir()).AppendRandom([]string{"ghost", "new"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}
	if err := NewMonitorQueue(s.StateDir(), "DP-1").AppendRandom([]string{"ghost", "new"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if _, err := s.CleanupImagesOlderThanDays(7); err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}

	history, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if slices.Contains(history.Images, "ghost") {
		t.Errorf("history still lists the removed file: %v", history.Images)
	}

	for name, q := range map[string]*Queue{
		"global": NewQueue(s.StateDir()),
		"DP-1":   NewMonitorQueue(s.StateDir(), "DP-1"),
	} {
		if err = q.Load(); err != nil {
			t.Fatalf("Load(%s) returned error: %v", name, err)
		}
		if slices.Contains(q.GetAll(), "ghost") {
			t.Errorf("%s queue still lists the removed file: %v", name, q.GetAll())
		}
	}
}

func TestCleanupLeavesBlacklistUntouched(t *testing.T) {
	f := newCleanupFixture(t)
	s := f.s

	// "old" is blacklisted and also gets cleaned up; "banned" has no image at
	// all. Both must stay blacklisted so the fetcher never downloads them again.
	if err := s.SaveBlacklist(&Blacklist{IDs: []string{"old", "banned"}}); err != nil {
		t.Fatalf("SaveBlacklist() returned error: %v", err)
	}

	if _, err := s.CleanupImagesOlderThanDays(7); err != nil {
		t.Fatalf("CleanupImagesOlderThanDays() returned error: %v", err)
	}

	blacklist, err := s.LoadBlacklist()
	if err != nil {
		t.Fatalf("LoadBlacklist() returned error: %v", err)
	}
	if !blacklist.Contains("old") || !blacklist.Contains("banned") || len(blacklist.IDs) != 2 {
		t.Fatalf("blacklist changed by cleanup: %v", blacklist.IDs)
	}
}
