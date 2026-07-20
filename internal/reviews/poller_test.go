package reviews

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu         sync.Mutex
	exists     bool
	snapshot   Snapshot
	saves      int
	saveCalls  int
	failOnSave int
	saveErr    error
}

func (s *memoryStore) Load(string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.exists {
		return Snapshot{}, os.ErrNotExist
	}
	return cloneSnapshot(s.snapshot), nil
}

func (s *memoryStore) Save(_ string, snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil && (s.failOnSave == 0 || s.saveCalls == s.failOnSave) {
		return s.saveErr
	}
	s.exists = true
	s.snapshot = cloneSnapshot(snapshot)
	s.saves++
	return nil
}

type scriptedFeed struct {
	mu     sync.Mutex
	pages  map[int]FeedPage
	errors map[int]error
	calls  []int
}

func (f *scriptedFeed) FetchPage(_ context.Context, _ AppConfig, page int) (FeedPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, page)
	if err := f.errors[page]; err != nil {
		return FeedPage{}, err
	}
	return f.pages[page], nil
}

func TestPollerInitialImportScansPastEmptyPage(t *testing.T) {
	app := testApp()
	app.MaxPages = 3
	store := &memoryStore{}
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("new", 3), testReview("older", 2)}},
		2: {Reviews: nil},
		3: {Reviews: []Review{testReview("after-hole", 1)}},
	}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	got := poller.Snapshot()
	if len(got.Reviews) != 3 || got.Sync.Status != "current" || got.Sync.HistoryGap != nil || got.Sync.HistoryLimit != nil || got.Sync.LastSuccessAt == nil || got.Sync.LastError != nil {
		t.Fatalf("unexpected initial snapshot: %#v", got)
	}
	if !reflect.DeepEqual(feed.calls, []int{1, 2, 3}) {
		t.Fatalf("pages fetched %v, want [1 2 3]", feed.calls)
	}
}

func TestPollerFindsRestartCheckpointAfterEmptyPage(t *testing.T) {
	app := testApp()
	app.MaxPages = 5
	checkpoint := testReview("checkpoint", 1)
	store := storeWithReviews(app, checkpoint)
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("newest", 5)}},
		2: {Reviews: []Review{testReview("newer", 4)}},
		3: {Reviews: nil},
		4: {Reviews: []Review{testReview("after-hole", 2), checkpoint}},
		5: {Reviews: []Review{testReview("must-not-fetch", 0)}},
	}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	got := poller.Snapshot()
	if !reflect.DeepEqual(feed.calls, []int{1, 2, 3, 4}) {
		t.Fatalf("pages fetched %v, want [1 2 3 4]", feed.calls)
	}
	if len(got.Reviews) != 4 || got.Sync.Status != "current" || got.Sync.HistoryGap != nil {
		t.Fatalf("checkpoint after empty page caused false gap: %#v", got)
	}
}

func TestPollerCatchUpStopsAfterCompleteOverlapPage(t *testing.T) {
	app := testApp()
	app.MaxPages = 5
	old := testReview("checkpoint", 1)
	store := storeWithReviews(app, old)
	updatedOld := old
	updatedOld.Content = "edited content"
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("new", 3)}},
		2: {Reviews: []Review{updatedOld, testReview("same-page", 2)}},
		3: {Reviews: []Review{testReview("must-not-fetch", 0)}},
	}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	got := poller.Snapshot()
	if !reflect.DeepEqual(feed.calls, []int{1, 2}) {
		t.Fatalf("pages fetched %v, want [1 2]", feed.calls)
	}
	if len(got.Reviews) != 3 || got.Sync.HistoryGap != nil || got.Sync.Status != "current" {
		t.Fatalf("unexpected catch-up result: %#v", got)
	}
	for _, review := range got.Reviews {
		if review.ID == old.ID && review.Content != "edited content" {
			t.Fatalf("same-ID review was not refreshed: %#v", review)
		}
	}
}

func TestPollerPageFailurePreservesCommittedPageProgress(t *testing.T) {
	app := testApp()
	old := testReview("checkpoint", 1)
	store := storeWithReviews(app, old)
	feed := &scriptedFeed{
		pages:  map[int]FeedPage{1: {Reviews: []Review{testReview("new", 2)}}},
		errors: map[int]error{2: errors.New("network down")},
	}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	got := poller.Snapshot()
	if len(got.Reviews) != 2 || got.Reviews[0].ID != "new" || got.Reviews[1].ID != old.ID {
		t.Fatalf("completed page was not preserved: %#v", got.Reviews)
	}
	if got.Sync.Status != "error" || got.Sync.LastError == nil || *got.Sync.LastError == "" || got.Sync.LastSuccessAt != nil || got.Sync.CatchUp == nil {
		t.Fatalf("unexpected failure metadata: %#v", got.Sync)
	}
	if got.Sync.CatchUp.CheckpointReviewID == nil || *got.Sync.CatchUp.CheckpointReviewID != old.ID {
		t.Fatalf("original checkpoint was not preserved: %#v", got.Sync.CatchUp)
	}
	if len(store.snapshot.Reviews) != 2 || store.snapshot.Sync.CatchUp == nil {
		t.Fatalf("page progress did not reach persisted state: %#v", store.snapshot)
	}
}

func TestPollerRestartContinuesTowardOriginalCheckpoint(t *testing.T) {
	app := testApp()
	app.MaxPages = 4
	checkpoint := testReview("checkpoint", 1)
	store := storeWithReviews(app, checkpoint)
	firstFeed := &scriptedFeed{
		pages:  map[int]FeedPage{1: {Reviews: []Review{testReview("page-one", 5)}}},
		errors: map[int]error{2: errors.New("interrupted")},
	}
	first := newTestPoller(t, app, store, firstFeed)
	first.PollOnce(context.Background())
	partial := first.Snapshot()
	if len(partial.Reviews) != 2 || partial.Sync.CatchUp == nil || partial.Sync.CatchUp.CheckpointReviewID == nil || *partial.Sync.CatchUp.CheckpointReviewID != checkpoint.ID {
		t.Fatalf("partial catch-up lost original checkpoint: %#v", partial)
	}

	secondFeed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("page-one", 5)}},
		2: {Reviews: []Review{testReview("page-two", 4)}},
		3: {Reviews: []Review{checkpoint}},
		4: {Reviews: []Review{testReview("must-not-fetch", 0)}},
	}}
	restarted := newTestPoller(t, app, store, secondFeed)
	restarted.PollOnce(context.Background())
	got := restarted.Snapshot()
	if !reflect.DeepEqual(secondFeed.calls, []int{1, 2, 3}) {
		t.Fatalf("restart stopped on a newly staged id: pages=%v", secondFeed.calls)
	}
	if len(got.Reviews) != 3 || got.Sync.CatchUp != nil || got.Sync.Status != "current" || got.Sync.HistoryGap != nil {
		t.Fatalf("restart did not complete against original checkpoint: %#v", got)
	}
}

func TestPollerDetectsAndPreservesHistoryGap(t *testing.T) {
	app := testApp()
	app.MaxPages = 2
	old := testReview("old-checkpoint", 1)
	store := storeWithReviews(app, old)
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("latest", 5)}},
		2: {Reviews: nil},
	}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	first := poller.Snapshot()
	if first.Sync.Status != "gap_detected" || first.Sync.HistoryGap == nil {
		t.Fatalf("gap was not detected: %#v", first.Sync)
	}
	if !first.Sync.HistoryGap.After.Equal(old.SubmittedAt) || !first.Sync.HistoryGap.Before.Equal(testReview("x", 5).SubmittedAt) {
		t.Fatalf("unexpected gap bounds: %#v", first.Sync.HistoryGap)
	}
	detectedAt := first.Sync.HistoryGap.DetectedAt

	feed.pages = map[int]FeedPage{1: {Reviews: []Review{testReview("latest", 5)}}}
	feed.calls = nil
	poller.PollOnce(context.Background())
	second := poller.Snapshot()
	if second.Sync.Status != "gap_detected" || second.Sync.HistoryGap == nil || !second.Sync.HistoryGap.DetectedAt.Equal(detectedAt) {
		t.Fatalf("durable gap was cleared or replaced: %#v", second.Sync)
	}
}

func TestPollerInitialImportRecordsAndPreservesHistoryLimit(t *testing.T) {
	app := testApp()
	app.MaxPages = 3
	store := &memoryStore{}
	finalPage := testReviews("final-", appleReviewPageCapacity, 1)
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("newest", 5)}},
		2: {Reviews: nil},
		3: {Reviews: finalPage},
	}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())
	first := poller.Snapshot()
	if first.Sync.HistoryGap != nil || first.Sync.Status != "current" || first.Sync.HistoryLimit == nil {
		t.Fatalf("initial import did not record bounded history: %#v", first.Sync)
	}
	if !first.Sync.HistoryLimit.OldestAvailableAt.Equal(oldestTime(finalPage)) {
		t.Fatalf("unexpected oldest available timestamp: %#v", first.Sync.HistoryLimit)
	}
	detectedAt := first.Sync.HistoryLimit.DetectedAt
	poller.PollOnce(context.Background())
	second := poller.Snapshot()
	if second.Sync.HistoryLimit == nil || !second.Sync.HistoryLimit.DetectedAt.Equal(detectedAt) {
		t.Fatalf("later success cleared history limit: %#v", second.Sync)
	}
}

func TestPollerInitialImportDoesNotRecordHistoryLimitWhenFinalPageIsPartial(t *testing.T) {
	app := testApp()
	app.MaxPages = 2
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("one", 1)}},
		2: {Reviews: []Review{testReview("partial-final-page", 0)}},
	}}
	poller := newTestPoller(t, app, &memoryStore{}, feed)
	poller.PollOnce(context.Background())
	if got := poller.Snapshot(); got.Sync.HistoryLimit != nil || got.Sync.HistoryGap != nil || got.Sync.Status != "current" {
		t.Fatalf("empty final page incorrectly reported bounded history: %#v", got.Sync)
	}
}

func TestPollerCancelledCatchUpPreservesCommittedPages(t *testing.T) {
	app := testApp()
	old := testReview("old", 1)
	store := storeWithReviews(app, old)
	feed := &scriptedFeed{
		pages:  map[int]FeedPage{1: {Reviews: []Review{testReview("new", 2)}}},
		errors: map[int]error{2: context.Canceled},
	}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())
	if got := poller.Snapshot(); len(got.Reviews) != 2 || got.Reviews[0].ID != "new" || got.Sync.Status != "error" || got.Sync.CatchUp == nil {
		t.Fatalf("cancelled catch-up lost completed progress: %#v", got)
	}
}

func TestPollerCatchUpStartSaveFailurePublishesInMemoryError(t *testing.T) {
	app := testApp()
	old := testReview("old", 1)
	store := storeWithReviews(app, old)
	store.saveErr = errors.New("disk full")
	feed := &scriptedFeed{pages: map[int]FeedPage{1: {Reviews: []Review{testReview("new", 2), old}}}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())
	got := poller.Snapshot()
	if len(got.Reviews) != 1 || got.Reviews[0].ID != old.ID || got.Sync.Status != "error" || got.Sync.LastError == nil || !strings.Contains(*got.Sync.LastError, "persist catch-up start") || got.Sync.CatchUp != nil {
		t.Fatalf("catch-up start failure was not exposed safely: %#v", got)
	}
	if store.snapshot.Sync.Status != "current" || len(store.snapshot.Reviews) != 1 || store.snapshot.Reviews[0].ID != old.ID {
		t.Fatalf("failed catch-up start changed durable state: %#v", store.snapshot)
	}
	if len(feed.calls) != 0 {
		t.Fatalf("feed was called after catch-up start could not be saved: %v", feed.calls)
	}
}

func TestPollerErrorSnapshotSaveFailurePublishesInMemoryError(t *testing.T) {
	app := testApp()
	old := testReview("old", 1)
	store := storeWithReviews(app, old)
	store.saveErr = errors.New("disk unavailable")
	store.failOnSave = 2 // catch-up start succeeds; saving the upstream error fails
	feed := &scriptedFeed{errors: map[int]error{1: errors.New("upstream unavailable")}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	got := poller.Snapshot()
	if len(got.Reviews) != 1 || got.Reviews[0].ID != old.ID || got.Sync.Status != "error" || got.Sync.LastError == nil || !strings.Contains(*got.Sync.LastError, "upstream unavailable") || got.Sync.CatchUp == nil {
		t.Fatalf("error-save failure was not exposed from durable state: %#v", got)
	}
	if store.snapshot.Sync.Status != "catching_up" || store.snapshot.Sync.CatchUp == nil || len(store.snapshot.Reviews) != 1 || store.snapshot.Reviews[0].ID != old.ID {
		t.Fatalf("failed error save changed durable state: %#v", store.snapshot)
	}
}

func TestPollerPageSaveFailureNeverExposesUnsavedPage(t *testing.T) {
	app := testApp()
	app.MaxPages = 3
	old := testReview("old", 1)
	store := storeWithReviews(app, old)
	store.saveErr = errors.New("disk full")
	store.failOnSave = 3 // catch-up start, page 1, then fail page 2
	feed := &scriptedFeed{pages: map[int]FeedPage{
		1: {Reviews: []Review{testReview("saved", 3)}},
		2: {Reviews: []Review{testReview("unsaved", 2)}},
		3: {Reviews: []Review{old}},
	}}
	poller := newTestPoller(t, app, store, feed)
	poller.PollOnce(context.Background())

	got := poller.Snapshot()
	if len(got.Reviews) != 2 || got.Reviews[0].ID != "saved" || got.Reviews[1].ID != old.ID {
		t.Fatalf("failed page became visible: %#v", got.Reviews)
	}
	if got.Sync.Status != "error" || got.Sync.CatchUp == nil {
		t.Fatalf("last durable progress was not retained: %#v", got.Sync)
	}
}

func TestPollerDoesNotOverlapConcurrentPolls(t *testing.T) {
	app := testApp()
	entered := make(chan struct{})
	release := make(chan struct{})
	feed := &blockingFeed{entered: entered, release: release}
	poller := newTestPoller(t, app, &memoryStore{}, feed)

	done := make(chan struct{})
	go func() {
		defer close(done)
		poller.PollOnce(context.Background())
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first poll did not reach feed")
	}

	secondDone := make(chan struct{})
	go func() {
		poller.PollOnce(context.Background())
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("overlapping PollOnce blocked instead of being skipped")
	}
	if calls := feed.calls.Load(); calls != 1 {
		t.Fatalf("feed was called %d times during overlap, want 1", calls)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first poll did not finish")
	}
}

func TestPollerRunWaitsForIntervalAfterPollCompletes(t *testing.T) {
	app := testApp()
	app.MaxPages = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	feed := &blockingFeed{entered: entered, release: release}
	poller := newTestPoller(t, app, &memoryStore{}, feed)
	waitStarted := make(chan time.Duration, 1)
	poller.wait = func(ctx context.Context, delay time.Duration) error {
		waitStarted <- delay
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		poller.Run(ctx)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("initial poll did not reach feed")
	}
	select {
	case <-waitStarted:
		t.Fatal("poll interval started before the active poll completed")
	default:
	}

	close(release)
	select {
	case delay := <-waitStarted:
		if delay != app.PollInterval {
			t.Fatalf("wait delay = %v, want %v", delay, app.PollInterval)
		}
	case <-time.After(time.Second):
		t.Fatal("poll interval did not start after poll completion")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}

type blockingFeed struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (f *blockingFeed) FetchPage(ctx context.Context, _ AppConfig, _ int) (FeedPage, error) {
	if f.calls.Add(1) == 1 {
		close(f.entered)
	}
	select {
	case <-f.release:
		return FeedPage{}, nil
	case <-ctx.Done():
		return FeedPage{}, ctx.Err()
	}
}

func newTestPoller(t *testing.T, app AppConfig, store ReviewStore, feed FeedClient) *Poller {
	t.Helper()
	poller, err := NewPoller(app, store, feed, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewPoller returned error: %v", err)
	}
	poller.now = func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }
	return poller
}

func storeWithReviews(app AppConfig, items ...Review) *memoryStore {
	snapshot := newSnapshot(app)
	snapshot.Reviews = append([]Review(nil), items...)
	snapshot.Sync.Status = "current"
	return &memoryStore{exists: true, snapshot: snapshot}
}

func testReview(id string, hour int) Review {
	return Review{
		ID: id, AppKey: "spotify-us", Title: "Title " + id, Content: "Content " + id,
		Author: "Author " + id, Score: 4,
		SubmittedAt: time.Date(2026, 7, 17, hour, 0, 0, 0, time.UTC),
		FetchedAt:   time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC),
	}
}

func testReviews(prefix string, count, hour int) []Review {
	reviews := make([]Review, 0, count)
	for i := 0; i < count; i++ {
		review := testReview(prefix+strconv.Itoa(i), hour)
		review.SubmittedAt = review.SubmittedAt.Add(-time.Duration(i) * time.Minute)
		reviews = append(reviews, review)
	}
	return reviews
}
