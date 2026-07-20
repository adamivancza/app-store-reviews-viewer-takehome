// Package reviews fetches, stores, and serves App Store customer reviews.
package reviews

import "time"

const snapshotVersion = 1

// SyncStatus describes the poller's durable synchronization state.
type SyncStatus string

// Synchronization states persisted with each app snapshot.
const (
	SyncStatusCatchingUp SyncStatus = "catching_up"
	SyncStatusCurrent    SyncStatus = "current"
	SyncStatusGap        SyncStatus = "gap_detected"
	SyncStatusError      SyncStatus = "error"
)

// AppConfig identifies the single app currently served. It is passed explicitly
// through the feed, polling, and storage layers so adding more pollers later does
// not require changing those components.
type AppConfig struct {
	Key          string        `json:"key"`
	Name         string        `json:"name"`
	AppID        string        `json:"appId"`
	Country      string        `json:"country"`
	PollInterval time.Duration `json:"-"`
	MaxPages     int           `json:"-"`
	DataDir      string        `json:"-"`
	ListenAddr   string        `json:"-"`
}

// Review is a normalized customer review fetched from the App Store.
type Review struct {
	ID          string    `json:"id"`
	AppKey      string    `json:"appKey"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      string    `json:"author"`
	Score       int       `json:"score"`
	SubmittedAt time.Time `json:"submittedAt"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

// HistoryGap is durable: once continuity with an older checkpoint could not be
// proven, later successful polls do not erase the warning.
type HistoryGap struct {
	DetectedAt time.Time `json:"detectedAt"`
	After      time.Time `json:"after"`
	Before     time.Time `json:"before"`
}

// CatchUpProgress records the boundary that existed before a multi-page poll
// began. It is persisted with every fetched page so a restart does not mistake
// a newly saved review for the original continuity checkpoint.
type CatchUpProgress struct {
	StartedAt             time.Time  `json:"startedAt"`
	CheckpointReviewID    *string    `json:"checkpointReviewId"`
	CheckpointSubmittedAt *time.Time `json:"checkpointSubmittedAt"`
	OldestFetchedAt       *time.Time `json:"oldestFetchedAt"`
}

// HistoryLimit records that a first import filled the final configured RSS
// page. Apple may have older reviews beyond the bounded public feed.
type HistoryLimit struct {
	DetectedAt        time.Time `json:"detectedAt"`
	OldestAvailableAt time.Time `json:"oldestAvailableAt"`
}

// SyncState records the durable progress and outcome of polling an app.
type SyncState struct {
	Status        SyncStatus       `json:"status"`
	LastAttemptAt *time.Time       `json:"lastAttemptAt"`
	LastSuccessAt *time.Time       `json:"lastSuccessAt"`
	LastError     *string          `json:"lastError"`
	HistoryGap    *HistoryGap      `json:"historyGap"`
	CatchUp       *CatchUpProgress `json:"catchUp"`
	HistoryLimit  *HistoryLimit    `json:"historyLimit"`
}

// Snapshot is the complete durable state for one configured app.
type Snapshot struct {
	Version int       `json:"version"`
	App     AppConfig `json:"app"`
	Reviews []Review  `json:"reviews"`
	Sync    SyncState `json:"sync"`
}

func newSnapshot(app AppConfig) Snapshot {
	return Snapshot{
		Version: snapshotVersion,
		App:     app,
		Reviews: []Review{},
		Sync:    SyncState{Status: SyncStatusCatchingUp},
	}
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := in
	out.Reviews = append([]Review(nil), in.Reviews...)
	if in.Sync.LastAttemptAt != nil {
		v := *in.Sync.LastAttemptAt
		out.Sync.LastAttemptAt = &v
	}
	if in.Sync.LastSuccessAt != nil {
		v := *in.Sync.LastSuccessAt
		out.Sync.LastSuccessAt = &v
	}
	if in.Sync.LastError != nil {
		v := *in.Sync.LastError
		out.Sync.LastError = &v
	}
	if in.Sync.HistoryGap != nil {
		v := *in.Sync.HistoryGap
		out.Sync.HistoryGap = &v
	}
	if in.Sync.CatchUp != nil {
		v := *in.Sync.CatchUp
		if in.Sync.CatchUp.CheckpointReviewID != nil {
			id := *in.Sync.CatchUp.CheckpointReviewID
			v.CheckpointReviewID = &id
		}
		if in.Sync.CatchUp.CheckpointSubmittedAt != nil {
			at := *in.Sync.CatchUp.CheckpointSubmittedAt
			v.CheckpointSubmittedAt = &at
		}
		if in.Sync.CatchUp.OldestFetchedAt != nil {
			at := *in.Sync.CatchUp.OldestFetchedAt
			v.OldestFetchedAt = &at
		}
		out.Sync.CatchUp = &v
	}
	if in.Sync.HistoryLimit != nil {
		v := *in.Sync.HistoryLimit
		out.Sync.HistoryLimit = &v
	}
	return out
}

func publicApp(app AppConfig) AppConfig {
	return AppConfig{Key: app.Key, Name: app.Name, AppID: app.AppID, Country: app.Country}
}
