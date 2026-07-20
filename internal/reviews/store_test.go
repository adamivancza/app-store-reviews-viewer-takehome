package reviews

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONStoreAtomicRoundTripAndRestart(t *testing.T) {
	dir := t.TempDir()
	app := testApp()
	store := NewJSONStore(dir)
	snapshot := newSnapshot(app)
	snapshot.Reviews = []Review{testReview("older", 1), testReview("newer", 3)}
	snapshot.Sync.Status = "current"
	snapshot.Sync.HistoryLimit = &HistoryLimit{
		DetectedAt:        testReview("newer", 3).FetchedAt,
		OldestAvailableAt: testReview("older", 1).SubmittedAt,
	}
	if err := store.Save(app.Key, snapshot); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := NewJSONStore(dir).Load(app.Key)
	if err != nil {
		t.Fatalf("Load after restart returned error: %v", err)
	}
	if len(loaded.Reviews) != 2 || loaded.Reviews[0].ID != "newer" || loaded.App.Key != app.Key || loaded.Sync.HistoryLimit == nil {
		t.Fatalf("unexpected loaded snapshot: %#v", loaded)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v, error %v", leftovers, err)
	}
}

func TestJSONStoreEnforcesSameConfiguredSizeLimitOnSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	app := testApp()
	snapshot := newSnapshot(app)
	largeReview := testReview("large", 1)
	largeReview.Content = strings.Repeat("x", 2048)
	snapshot.Reviews = []Review{largeReview}
	snapshot.Sync.Status = "current"

	smallStore := NewJSONStoreWithMaxSize(dir, 1024)
	if err := smallStore.Save(app.Key, snapshot); err == nil || !strings.Contains(err.Error(), "exceeds maximum size of 1024 bytes") {
		t.Fatalf("oversized Save returned %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, app.Key+".json")); !os.IsNotExist(err) {
		t.Fatalf("rejected save created a snapshot: %v", err)
	}

	largeStore := NewJSONStoreWithMaxSize(dir, 1<<20)
	if err := largeStore.Save(app.Key, snapshot); err != nil {
		t.Fatalf("large configured Save returned error: %v", err)
	}
	if _, err := smallStore.Load(app.Key); err == nil || !strings.Contains(err.Error(), "exceeds maximum size of 1024 bytes") {
		t.Fatalf("oversized Load returned %v", err)
	}
}

func TestJSONStoreCorruptStateFailsClearly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spotify-us.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"app":`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewJSONStore(dir).Load("spotify-us")
	if err == nil || !strings.Contains(err.Error(), "decode snapshot") {
		t.Fatalf("got error %v, want clear decode failure", err)
	}

	_, err = NewPoller(testApp(), NewJSONStore(dir), &scriptedFeed{}, nil)
	if err == nil || !strings.Contains(err.Error(), "load persisted reviews") {
		t.Fatalf("startup accepted corrupt state: %v", err)
	}
}

func TestJSONStoreRejectsInvalidAndMismatchedState(t *testing.T) {
	store := NewJSONStore(t.TempDir())
	snapshot := newSnapshot(testApp())
	snapshot.Reviews = []Review{testReview("bad", 1)}
	snapshot.Reviews[0].Score = 0
	if err := store.Save("spotify-us", snapshot); err == nil {
		t.Fatal("invalid review was persisted")
	}
	if err := store.Save("../escape", newSnapshot(testApp())); err == nil {
		t.Fatal("unsafe app key was accepted")
	}
}

func TestNewPollerRestoresReviewsWithoutPolling(t *testing.T) {
	dir := t.TempDir()
	app := testApp()
	store := NewJSONStore(dir)
	snapshot := newSnapshot(app)
	snapshot.Reviews = []Review{testReview("persisted", 1)}
	snapshot.Sync.Status = "current"
	if err := store.Save(app.Key, snapshot); err != nil {
		t.Fatal(err)
	}

	feed := &scriptedFeed{}
	poller, err := NewPoller(app, NewJSONStore(dir), feed, nil)
	if err != nil {
		t.Fatalf("NewPoller returned error: %v", err)
	}
	got := poller.Snapshot()
	if len(got.Reviews) != 1 || got.Reviews[0].ID != "persisted" || len(feed.calls) != 0 {
		t.Fatalf("restart did not serve persisted state immediately: %#v, calls %v", got, feed.calls)
	}
}

func TestPartialCatchUpSurvivesJSONStoreRestart(t *testing.T) {
	dir := t.TempDir()
	app := testApp()
	app.DataDir = dir
	app.MaxPages = 3
	checkpoint := testReview("checkpoint", 1)
	store := NewJSONStore(dir)
	baseline := newSnapshot(app)
	baseline.Reviews = []Review{checkpoint}
	baseline.Sync.Status = "current"
	if err := store.Save(app.Key, baseline); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	interruptedFeed := &scriptedFeed{
		pages:  map[int]FeedPage{1: {Reviews: []Review{testReview("new", 3)}}},
		errors: map[int]error{2: errors.New("upstream interrupted")},
	}
	first := newTestPoller(t, app, store, interruptedFeed)
	first.PollOnce(context.Background())

	restartedFeed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("new", 3)}},
		2: {Reviews: []Review{testReview("newer-than-checkpoint", 2), checkpoint}},
	}}
	restarted := newTestPoller(t, app, NewJSONStore(dir), restartedFeed)
	beforeRetry := restarted.Snapshot()
	if len(beforeRetry.Reviews) != 2 || beforeRetry.Sync.CatchUp == nil || beforeRetry.Sync.CatchUp.CheckpointReviewID == nil || *beforeRetry.Sync.CatchUp.CheckpointReviewID != checkpoint.ID {
		t.Fatalf("restart lost durable progress: %#v", beforeRetry)
	}
	restarted.PollOnce(context.Background())
	got := restarted.Snapshot()
	if len(restartedFeed.calls) != 2 || len(got.Reviews) != 3 || got.Sync.CatchUp != nil || got.Sync.Status != "current" {
		t.Fatalf("retry did not finish at original checkpoint: calls=%v snapshot=%#v", restartedFeed.calls, got)
	}
}

func TestPollResultAndHistoryGapSurviveRealRestart(t *testing.T) {
	dir := t.TempDir()
	app := testApp()
	app.DataDir = dir
	app.MaxPages = 2
	store := NewJSONStore(dir)
	baseline := newSnapshot(app)
	baseline.Reviews = []Review{testReview("old-checkpoint", 1)}
	baseline.Sync.Status = "current"
	if err := store.Save(app.Key, baseline); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	fixtureFeed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("new-review", 5)}},
		2: {Reviews: nil},
	}}
	first := newTestPoller(t, app, store, fixtureFeed)
	first.PollOnce(context.Background())
	if got := first.Snapshot(); got.Sync.Status != "gap_detected" || got.Sync.HistoryGap == nil || len(got.Reviews) != 2 {
		t.Fatalf("poll did not create expected durable state: %#v", got)
	}

	unusedFeed := &scriptedFeed{}
	restarted, err := NewPoller(app, NewJSONStore(dir), unusedFeed, nil)
	if err != nil {
		t.Fatalf("restart NewPoller: %v", err)
	}
	got := restarted.Snapshot()
	if len(got.Reviews) != 2 || got.Sync.Status != "gap_detected" || got.Sync.HistoryGap == nil || len(unusedFeed.calls) != 0 {
		t.Fatalf("restart did not restore poll result before network access: %#v, calls=%v", got, unusedFeed.calls)
	}
}
