package reviews

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// ReviewStore loads and atomically saves the state of one app.
//
// Poller depends on this contract rather than JSONStore, so another durable
// implementation can replace JSONStore without changing polling logic.
type ReviewStore interface {
	Load(appKey string) (Snapshot, error)
	Save(appKey string, snapshot Snapshot) error
}

// JSONStore persists each app snapshot as one JSON file.
type JSONStore struct {
	dir              string
	maxSnapshotBytes int64
}

const defaultMaxSnapshotBytes int64 = 64 << 20

// NewJSONStore creates a JSON-backed review store in dir.
func NewJSONStore(dir string) *JSONStore {
	return NewJSONStoreWithMaxSize(dir, defaultMaxSnapshotBytes)
}

// NewJSONStoreWithMaxSize creates a JSON store with an explicit snapshot limit.
func NewJSONStoreWithMaxSize(dir string, maxSnapshotBytes int64) *JSONStore {
	if maxSnapshotBytes <= 0 {
		maxSnapshotBytes = defaultMaxSnapshotBytes
	}
	return &JSONStore{dir: dir, maxSnapshotBytes: maxSnapshotBytes}
}

func (s *JSONStore) path(appKey string) (string, error) {
	if !appKeyPattern.MatchString(appKey) {
		return "", errors.New("invalid app key")
	}
	return filepath.Join(s.dir, appKey+".json"), nil
}

// Load reads and validates the snapshot for appKey.
func (s *JSONStore) Load(appKey string) (Snapshot, error) {
	path, err := s.path(appKey)
	if err != nil {
		return Snapshot{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Snapshot{}, fmt.Errorf("stat snapshot %s: %w", path, err)
	}
	if info.Size() > s.maxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("snapshot %s exceeds maximum size of %d bytes", path, s.maxSnapshotBytes)
	}

	var snapshot Snapshot
	dec := json.NewDecoder(io.LimitReader(f, s.maxSnapshotBytes+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot %s: %w", path, err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot %s: %w", path, err)
	}
	if err := validateSnapshot(appKey, snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("validate snapshot %s: %w", path, err)
	}
	sortReviews(snapshot.Reviews)
	return snapshot, nil
}

// Save atomically replaces the snapshot for appKey.
func (s *JSONStore) Save(appKey string, snapshot Snapshot) error {
	path, err := s.path(appKey)
	if err != nil {
		return err
	}
	if err := validateSnapshot(appKey, snapshot); err != nil {
		return fmt.Errorf("refuse to save invalid snapshot: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	data = append(data, '\n')
	if int64(len(data)) > s.maxSnapshotBytes {
		return fmt.Errorf("snapshot exceeds maximum size of %d bytes", s.maxSnapshotBytes)
	}
	tmp, err := os.CreateTemp(s.dir, "."+appKey+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary snapshot: %w", err)
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set snapshot permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close snapshot: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace snapshot: %w", err)
	}
	removeTemp = false

	// Best-effort directory sync makes the rename durable on filesystems that
	// support syncing directories.
	if dir, err := os.Open(s.dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func validateSnapshot(appKey string, snapshot Snapshot) error {
	if snapshot.Version != snapshotVersion {
		return fmt.Errorf("unsupported version %d", snapshot.Version)
	}
	if snapshot.App.Key != appKey {
		return fmt.Errorf("snapshot app key %q does not match %q", snapshot.App.Key, appKey)
	}
	switch snapshot.Sync.Status {
	case SyncStatusCatchingUp, SyncStatusCurrent, SyncStatusGap, SyncStatusError:
	default:
		return fmt.Errorf("unknown sync status %q", snapshot.Sync.Status)
	}
	if snapshot.Sync.LastAttemptAt != nil && snapshot.Sync.LastAttemptAt.IsZero() {
		return errors.New("lastAttemptAt is invalid")
	}
	if snapshot.Sync.LastSuccessAt != nil && snapshot.Sync.LastSuccessAt.IsZero() {
		return errors.New("lastSuccessAt is invalid")
	}
	if gap := snapshot.Sync.HistoryGap; gap != nil && (gap.DetectedAt.IsZero() || gap.After.IsZero() || gap.Before.IsZero()) {
		return errors.New("historyGap contains an invalid timestamp")
	}
	if progress := snapshot.Sync.CatchUp; progress != nil {
		if snapshot.Sync.Status != SyncStatusCatchingUp && snapshot.Sync.Status != SyncStatusError {
			return errors.New("catchUp requires catching_up or error sync status")
		}
		if progress.StartedAt.IsZero() {
			return errors.New("catchUp startedAt is invalid")
		}
		if (progress.CheckpointReviewID == nil) != (progress.CheckpointSubmittedAt == nil) {
			return errors.New("catchUp checkpoint id and timestamp must both be set")
		}
		if progress.CheckpointReviewID != nil && (*progress.CheckpointReviewID == "" || progress.CheckpointSubmittedAt.IsZero()) {
			return errors.New("catchUp checkpoint is invalid")
		}
		if progress.OldestFetchedAt != nil && progress.OldestFetchedAt.IsZero() {
			return errors.New("catchUp oldestFetchedAt is invalid")
		}
	}
	if limit := snapshot.Sync.HistoryLimit; limit != nil && (limit.DetectedAt.IsZero() || limit.OldestAvailableAt.IsZero()) {
		return errors.New("historyLimit contains an invalid timestamp")
	}
	seen := make(map[string]struct{}, len(snapshot.Reviews))
	for i, review := range snapshot.Reviews {
		if review.ID == "" {
			return fmt.Errorf("review %d has no id", i)
		}
		if review.AppKey != appKey {
			return fmt.Errorf("review %q belongs to app %q", review.ID, review.AppKey)
		}
		if review.Score < 1 || review.Score > 5 {
			return fmt.Errorf("review %q has invalid score %d", review.ID, review.Score)
		}
		if review.SubmittedAt.IsZero() || review.FetchedAt.IsZero() {
			return fmt.Errorf("review %q has an invalid timestamp", review.ID)
		}
		key := reviewKey(review)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate review id %q", review.ID)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func sortReviews(items []Review) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SubmittedAt.Equal(items[j].SubmittedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].SubmittedAt.After(items[j].SubmittedAt)
	})
}
