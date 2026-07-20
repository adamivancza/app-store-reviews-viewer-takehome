package reviews

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOperationalEndpointsSeparateLifecycleAndDataState(t *testing.T) {
	app := testApp()
	initial := newSnapshot(app)
	initialAPI := NewAPI(&Poller{snapshot: initial})

	live := serveAPI(t, initialAPI, http.MethodGet, "/api/live")
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected liveness response: %d %s", live.Code, live.Body.String())
	}
	ready := serveAPI(t, initialAPI, http.MethodGet, "/api/ready")
	if ready.Code != http.StatusServiceUnavailable || !strings.Contains(ready.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("initial service should not be ready: %d %s", ready.Code, ready.Body.String())
	}
	freshness := serveAPI(t, initialAPI, http.MethodGet, "/api/freshness")
	initialFreshness := decodeOperationalResponse(t, freshness)
	if freshness.Code != http.StatusOK || initialFreshness.Status != "updating" || initialFreshness.Complete == nil || *initialFreshness.Complete {
		t.Fatalf("unexpected initial freshness: %d %s", freshness.Code, freshness.Body.String())
	}
	initialFailure := cloneSnapshot(initial)
	initialFailure.Sync.Status = "error"
	initialError := "upstream unavailable"
	initialFailure.Sync.LastError = &initialError
	unavailable := serveAPI(t, NewAPI(&Poller{snapshot: initialFailure}), http.MethodGet, "/api/freshness")
	if got := decodeOperationalResponse(t, unavailable); got.Status != "unavailable" || got.Complete == nil || *got.Complete {
		t.Fatalf("unexpected unavailable freshness: %s", unavailable.Body.String())
	}

	succeededAt := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	current := cloneSnapshot(initial)
	current.Sync.Status = "current"
	current.Sync.LastSuccessAt = &succeededAt
	currentAPI := NewAPI(&Poller{snapshot: current})
	ready = serveAPI(t, currentAPI, http.MethodGet, "/api/ready")
	if ready.Code != http.StatusOK || !strings.Contains(ready.Body.String(), `"status":"ready"`) {
		t.Fatalf("successful empty sync should be ready: %d %s", ready.Code, ready.Body.String())
	}
	freshness = serveAPI(t, currentAPI, http.MethodGet, "/api/freshness")
	currentFreshness := decodeOperationalResponse(t, freshness)
	if currentFreshness.Status != "current" || currentFreshness.Complete == nil || !*currentFreshness.Complete {
		t.Fatalf("unexpected current freshness: %s", freshness.Body.String())
	}

	failed := cloneSnapshot(current)
	failed.Sync.Status = "error"
	message := "upstream unavailable"
	failed.Sync.LastError = &message
	failedAPI := NewAPI(&Poller{snapshot: failed})
	ready = serveAPI(t, failedAPI, http.MethodGet, "/api/ready")
	if ready.Code != http.StatusOK {
		t.Fatalf("cached service should remain ready after an upstream failure: %d %s", ready.Code, ready.Body.String())
	}
	freshness = serveAPI(t, failedAPI, http.MethodGet, "/api/freshness")
	if got := decodeOperationalResponse(t, freshness); got.Status != "stale" {
		t.Fatalf("upstream failure should report stale data: %s", freshness.Body.String())
	}

	gap := cloneSnapshot(current)
	gap.Sync.Status = "gap_detected"
	gap.Sync.HistoryGap = &HistoryGap{
		DetectedAt: succeededAt,
		After:      succeededAt.Add(-48 * time.Hour),
		Before:     succeededAt.Add(-24 * time.Hour),
	}
	gapAPI := NewAPI(&Poller{snapshot: gap})
	freshness = serveAPI(t, gapAPI, http.MethodGet, "/api/freshness")
	gapFreshness := decodeOperationalResponse(t, freshness)
	if gapFreshness.Status != "current" || gapFreshness.Complete == nil || *gapFreshness.Complete {
		t.Fatalf("gap should separate freshness from completeness: %s", freshness.Body.String())
	}

	limited := cloneSnapshot(current)
	limited.Sync.HistoryLimit = &HistoryLimit{DetectedAt: succeededAt, OldestAvailableAt: succeededAt.Add(-30 * 24 * time.Hour)}
	limitedResponse := serveAPI(t, NewAPI(&Poller{snapshot: limited}), http.MethodGet, "/api/freshness")
	limitedFreshness := decodeOperationalResponse(t, limitedResponse)
	if limitedFreshness.Status != "current" || limitedFreshness.Complete == nil || *limitedFreshness.Complete {
		t.Fatalf("history limit should preserve freshness but mark coverage incomplete: %s", limitedResponse.Body.String())
	}
}

func TestReadyAcceptsUsefulCachedReviewsBeforeFirstRecordedSuccess(t *testing.T) {
	snapshot := newSnapshot(testApp())
	snapshot.Sync.Status = "error"
	snapshot.Reviews = []Review{testReview("cached", 1)}
	response := serveAPI(t, NewAPI(&Poller{snapshot: snapshot}), http.MethodGet, "/api/ready")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ready"`) {
		t.Fatalf("cached reviews should make the service ready: %d %s", response.Code, response.Body.String())
	}
}

type operationalResponse struct {
	Status   string `json:"status"`
	Complete *bool  `json:"complete"`
}

func decodeOperationalResponse(t *testing.T, response interface{ Result() *http.Response }) operationalResponse {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	var body operationalResponse
	if err := json.NewDecoder(result.Body).Decode(&body); err != nil {
		t.Fatalf("decode operational response: %v", err)
	}
	return body
}
