package reviews

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"
	"time"
)

// Poller synchronizes one app's review feed into durable storage.
type Poller struct {
	app      AppConfig
	store    ReviewStore
	feed     FeedClient
	logger   *slog.Logger
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
	mu       sync.RWMutex
	snapshot Snapshot
	pollMu   sync.Mutex
}

const appleReviewPageCapacity = 50

// NewPoller restores app state and creates its polling service.
func NewPoller(app AppConfig, store ReviewStore, feed FeedClient, logger *slog.Logger) (*Poller, error) {
	if store == nil || feed == nil {
		return nil, errors.New("store and feed client are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	snapshot, err := store.Load(app.Key)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load persisted reviews: %w", err)
		}
		snapshot = newSnapshot(app)
	} else {
		// Runtime settings come from current configuration; persisted identity
		// must still agree to avoid serving data for a different app.
		if snapshot.App.AppID != app.AppID || snapshot.App.Country != app.Country {
			return nil, fmt.Errorf("persisted app identity does not match configuration")
		}
		snapshot.App = app
	}
	return &Poller{
		app: app, store: store, feed: feed, logger: logger,
		now: time.Now, wait: waitForNextPoll, snapshot: snapshot,
	}, nil
}

// Snapshot returns an isolated copy of the poller's current state.
func (p *Poller) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneSnapshot(p.snapshot)
}

// Run polls immediately and then waits one configured interval after each poll.
func (p *Poller) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		p.PollOnce(ctx)
		wait := p.wait
		if wait == nil {
			wait = waitForNextPoll
		}
		if err := wait(ctx, p.app.PollInterval); err != nil {
			return
		}
	}
}

func waitForNextPoll(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// PollOnce is safe to call concurrently. A second invocation returns without
// starting another upstream request while the current poll is in progress.
func (p *Poller) PollOnce(ctx context.Context) {
	if !p.pollMu.TryLock() {
		p.logger.Debug("poll skipped because another poll is running", "app", p.app.Key)
		return
	}
	defer p.pollMu.Unlock()

	started := p.now().UTC()
	baseline := p.Snapshot()
	working := cloneSnapshot(baseline)
	if working.Sync.CatchUp == nil {
		working.Sync.CatchUp = newCatchUpProgress(baseline, started)
	}
	working.Sync.Status = SyncStatusCatchingUp
	working.Sync.LastAttemptAt = timePtr(started)
	working.Sync.LastError = nil
	if err := p.saveAndPublish(working); err != nil {
		p.publishError(baseline, started, fmt.Errorf("persist catch-up start: %w", err))
		p.logger.Error("could not persist catch-up start", "app", p.app.Key, "error", err)
		return
	}

	overlap := false
	pagesFetched := 0
	finalPageSaturated := false
	for page := 1; page <= p.app.MaxPages; page++ {
		feedPage, err := p.feed.FetchPage(ctx, p.app, page)
		if err != nil {
			p.recordFailure(working, started, err)
			p.logger.Error("review poll failed", "app", p.app.Key, "page", page, "duration", p.now().Sub(started), "error", err)
			return
		}
		pagesFetched++
		if page == p.app.MaxPages {
			finalPageSaturated = len(feedPage.Reviews) >= appleReviewPageCapacity
		}

		pageResult, pageOverlaps := stageFeedPage(working, feedPage, p.app.Key, started)
		overlap = pageOverlaps
		if err := p.saveAndPublish(pageResult); err != nil {
			p.recordFailure(working, started, fmt.Errorf("persist feed page %d: %w", page, err))
			p.logger.Error("review page could not be persisted", "app", p.app.Key, "page", page, "error", err)
			return
		}
		working = pageResult
		if overlap {
			break
		}
	}

	completed := completePoll(working, p.app, started, p.now().UTC(), overlap, finalPageSaturated)

	newCount := len(completed.Reviews) - len(baseline.Reviews)
	if err := p.saveAndPublish(completed); err != nil {
		p.recordFailure(working, started, fmt.Errorf("persist poll result: %w", err))
		p.logger.Error("review poll could not be persisted", "app", p.app.Key, "duration", p.now().Sub(started), "error", err)
		return
	}
	p.logger.Info("review poll completed", "app", p.app.Key, "pages", pagesFetched, "newReviews", newCount, "duration", p.now().Sub(started), "historyGap", completed.Sync.HistoryGap != nil)
}

func stageFeedPage(working Snapshot, feedPage FeedPage, appKey string, started time.Time) (Snapshot, bool) {
	progress := cloneCatchUp(working.Sync.CatchUp)
	overlap := false
	if progress.CheckpointReviewID != nil {
		for _, review := range feedPage.Reviews {
			if review.AppKey == appKey && review.ID == *progress.CheckpointReviewID {
				overlap = true
				break
			}
		}
	}
	updateOldestFetched(progress, feedPage.Reviews)

	result := cloneSnapshot(working)
	result.Reviews = mergeReviews(working.Reviews, feedPage.Reviews)
	result.Sync.CatchUp = progress
	result.Sync.Status = SyncStatusCatchingUp
	result.Sync.LastAttemptAt = timePtr(started)
	result.Sync.LastError = nil
	return result, overlap
}

func completePoll(working Snapshot, app AppConfig, started, completedAt time.Time, overlap, finalPageSaturated bool) Snapshot {
	completed := cloneSnapshot(working)
	completed.Version = snapshotVersion
	completed.App = app
	completed.Sync.Status = SyncStatusCurrent
	completed.Sync.LastAttemptAt = timePtr(started)
	completed.Sync.LastSuccessAt = timePtr(completedAt)
	completed.Sync.LastError = nil

	progress := completed.Sync.CatchUp
	if progress != nil && progress.CheckpointReviewID != nil && !overlap && completed.Sync.HistoryGap == nil {
		completed.Sync.HistoryGap = detectCheckpointGap(started, progress)
	}
	if progress != nil && progress.CheckpointReviewID == nil && finalPageSaturated && completed.Sync.HistoryLimit == nil && progress.OldestFetchedAt != nil {
		completed.Sync.HistoryLimit = &HistoryLimit{DetectedAt: completedAt, OldestAvailableAt: *progress.OldestFetchedAt}
	}
	completed.Sync.CatchUp = nil
	if completed.Sync.HistoryGap != nil {
		completed.Sync.Status = SyncStatusGap
	}
	return completed
}

func (p *Poller) recordFailure(baseline Snapshot, at time.Time, pollErr error) {
	failed := errorSnapshot(baseline, at, pollErr)
	if err := p.saveAndPublish(failed); err != nil {
		p.logger.Error("failed to persist poll error", "app", p.app.Key, "error", err)
		p.publish(failed)
	}
}

func (p *Poller) saveAndPublish(snapshot Snapshot) error {
	if err := p.store.Save(p.app.Key, snapshot); err != nil {
		return err
	}
	p.publish(snapshot)
	return nil
}

func (p *Poller) publishError(baseline Snapshot, at time.Time, pollErr error) {
	p.publish(errorSnapshot(baseline, at, pollErr))
}

func errorSnapshot(baseline Snapshot, at time.Time, pollErr error) Snapshot {
	failed := cloneSnapshot(baseline)
	failed.Sync.Status = SyncStatusError
	failed.Sync.LastAttemptAt = timePtr(at)
	message := pollErr.Error()
	failed.Sync.LastError = &message
	return failed
}

func (p *Poller) publish(snapshot Snapshot) {
	p.mu.Lock()
	p.snapshot = snapshot
	p.mu.Unlock()
}

func mergeReviews(existing, fetched []Review) []Review {
	byID := make(map[string]Review, len(existing)+len(fetched))
	for _, review := range existing {
		byID[reviewKey(review)] = review
	}
	for _, review := range fetched {
		byID[reviewKey(review)] = review
	}
	merged := make([]Review, 0, len(byID))
	for _, review := range byID {
		merged = append(merged, review)
	}
	sortReviews(merged)
	return merged
}

func reviewKey(review Review) string { return review.AppKey + "\x00" + review.ID }

func newCatchUpProgress(snapshot Snapshot, started time.Time) *CatchUpProgress {
	progress := &CatchUpProgress{StartedAt: started}
	if checkpoint, ok := newestReview(snapshot.Reviews); ok {
		id := checkpoint.ID
		submittedAt := checkpoint.SubmittedAt
		progress.CheckpointReviewID = &id
		progress.CheckpointSubmittedAt = &submittedAt
	}
	return progress
}

func cloneCatchUp(progress *CatchUpProgress) *CatchUpProgress {
	if progress == nil {
		return nil
	}
	cloned := *progress
	if progress.CheckpointReviewID != nil {
		id := *progress.CheckpointReviewID
		cloned.CheckpointReviewID = &id
	}
	if progress.CheckpointSubmittedAt != nil {
		at := *progress.CheckpointSubmittedAt
		cloned.CheckpointSubmittedAt = &at
	}
	if progress.OldestFetchedAt != nil {
		at := *progress.OldestFetchedAt
		cloned.OldestFetchedAt = &at
	}
	return &cloned
}

func updateOldestFetched(progress *CatchUpProgress, reviews []Review) {
	oldest := oldestTime(reviews)
	if oldest.IsZero() {
		return
	}
	if progress.OldestFetchedAt == nil || oldest.Before(*progress.OldestFetchedAt) {
		progress.OldestFetchedAt = timePtr(oldest)
	}
}

func detectCheckpointGap(detectedAt time.Time, progress *CatchUpProgress) *HistoryGap {
	after := detectedAt
	if progress.CheckpointSubmittedAt != nil {
		after = *progress.CheckpointSubmittedAt
	}
	before := detectedAt
	if progress.OldestFetchedAt != nil {
		before = *progress.OldestFetchedAt
	}
	return &HistoryGap{DetectedAt: detectedAt, After: after, Before: before}
}

func newestReview(reviews []Review) (Review, bool) {
	if len(reviews) == 0 {
		return Review{}, false
	}
	newest := reviews[0]
	for _, review := range reviews[1:] {
		if review.SubmittedAt.After(newest.SubmittedAt) || (review.SubmittedAt.Equal(newest.SubmittedAt) && review.ID < newest.ID) {
			newest = review
		}
	}
	return newest, true
}

func oldestTime(reviews []Review) time.Time {
	var result time.Time
	for _, review := range reviews {
		if result.IsZero() || review.SubmittedAt.Before(result) {
			result = review.SubmittedAt
		}
	}
	return result
}

func timePtr(v time.Time) *time.Time { return &v }
