package reviews

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReviewsAPIDefaultsWindowAndPagination(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	poller := apiTestPoller(
		reviewAt("newest", now.Add(-time.Hour)),
		reviewAt("boundary", now.Add(-48*time.Hour)),
		reviewAt("too-old", now.Add(-48*time.Hour-time.Nanosecond)),
		reviewAt("future", now.Add(time.Nanosecond)),
	)
	api := NewAPI(poller)
	api.now = func() time.Time { return now }

	response := serveAPI(t, api, http.MethodGet, "/api/reviews")
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	body := decodeReviewsResponse(t, response)
	if body.Window.Hours != 48 || !body.Window.From.Equal(now.Add(-48*time.Hour)) || !body.Window.To.Equal(now) {
		t.Fatalf("unexpected window: %#v", body.Window)
	}
	if body.Pagination != (apiPagination{Page: 1, PageSize: 25, TotalItems: 2, TotalPages: 1}) {
		t.Fatalf("unexpected default pagination: %#v", body.Pagination)
	}
	if !body.Coverage.Complete || body.Coverage.LimitedBefore != nil {
		t.Fatalf("unexpected default coverage: %#v", body.Coverage)
	}
	if len(body.Reviews) != 2 || body.Reviews[0].ID != "newest" || body.Reviews[1].ID != "boundary" {
		t.Fatalf("unexpected filtered reviews: %#v", body.Reviews)
	}
}

func TestReviewsAPIPaginatesNewestFirst(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	items := make([]Review, 0, 27)
	// Supply oldest-first to prove the poller's snapshot invariant is preserved by
	// filtering and pagination at the API boundary.
	for i := 26; i >= 0; i-- {
		items = append(items, reviewAt(fmt.Sprintf("review-%02d", i), now.Add(-time.Duration(i)*time.Minute)))
	}
	api := NewAPI(apiTestPoller(items...))
	api.now = func() time.Time { return now }

	defaultPage := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews"))
	if defaultPage.Pagination != (apiPagination{Page: 1, PageSize: 25, TotalItems: 27, TotalPages: 2}) || len(defaultPage.Reviews) != 25 {
		t.Fatalf("default page was not limited to 25 items: %#v, reviews=%d", defaultPage.Pagination, len(defaultPage.Reviews))
	}
	if defaultPage.Reviews[0].ID != "review-00" || defaultPage.Reviews[24].ID != "review-24" {
		t.Fatalf("default page is not newest-first: first=%q last=%q", defaultPage.Reviews[0].ID, defaultPage.Reviews[24].ID)
	}

	response := serveAPI(t, api, http.MethodGet, "/api/reviews?page=2&pageSize=10")
	body := decodeReviewsResponse(t, response)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if body.Pagination != (apiPagination{Page: 2, PageSize: 10, TotalItems: 27, TotalPages: 3}) {
		t.Fatalf("unexpected pagination: %#v", body.Pagination)
	}
	if len(body.Reviews) != 10 {
		t.Fatalf("page contains %d reviews, want 10", len(body.Reviews))
	}
	for i, review := range body.Reviews {
		want := fmt.Sprintf("review-%02d", i+10)
		if review.ID != want {
			t.Fatalf("review %d is %q, want %q", i, review.ID, want)
		}
	}

	lastPage := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?page=3&pageSize=10"))
	if len(lastPage.Reviews) != 7 || lastPage.Reviews[0].ID != "review-20" || lastPage.Reviews[6].ID != "review-26" {
		t.Fatalf("unexpected final page: %#v", lastPage.Reviews)
	}
}

func TestReviewsAPIFiltersScoresBeforePagination(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	items := make([]Review, 0, 30)
	for i := 0; i < 30; i++ {
		review := reviewAt(fmt.Sprintf("review-%02d", i), now.Add(-time.Duration(i)*time.Minute))
		review.Score = 1 + i%5
		items = append(items, review)
	}
	api := NewAPI(apiTestPoller(items...))
	api.now = func() time.Time { return now }

	response := serveAPI(t, api, http.MethodGet, "/api/reviews?scores=2,5&page=2&pageSize=5")
	body := decodeReviewsResponse(t, response)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if body.Pagination != (apiPagination{Page: 2, PageSize: 5, TotalItems: 12, TotalPages: 3}) {
		t.Fatalf("score filter was not applied before pagination: %#v", body.Pagination)
	}
	if len(body.Reviews) != 5 {
		t.Fatalf("filtered page contains %d reviews, want 5", len(body.Reviews))
	}
	for _, review := range body.Reviews {
		if review.Score != 2 && review.Score != 5 {
			t.Fatalf("unexpected score %d in filtered response: %#v", review.Score, body.Reviews)
		}
	}
	if body.Reviews[0].ID != "review-14" || body.Reviews[4].ID != "review-24" {
		t.Fatalf("filtered page is not newest-first: %#v", body.Reviews)
	}

	empty := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?scores="))
	if empty.Pagination.TotalItems != 0 || empty.Pagination.TotalPages != 0 || len(empty.Reviews) != 0 {
		t.Fatalf("an explicitly empty score selection should return no reviews: %#v", empty)
	}
}

func TestReviewsAPIEmptyAndOutOfRangePages(t *testing.T) {
	t.Run("empty result", func(t *testing.T) {
		body := decodeReviewsResponse(t, serveAPI(t, NewAPI(apiTestPoller()), http.MethodGet, "/api/reviews?page=1&pageSize=100"))
		if body.Pagination != (apiPagination{Page: 1, PageSize: 100, TotalItems: 0, TotalPages: 0}) {
			t.Fatalf("unexpected empty pagination: %#v", body.Pagination)
		}
		if body.Reviews == nil || len(body.Reviews) != 0 {
			t.Fatalf("empty result must encode as an empty list: %#v", body.Reviews)
		}
	})

	t.Run("positive page beyond total", func(t *testing.T) {
		api := NewAPI(apiTestPoller(testReview("one", 1), testReview("two", 2)))
		api.now = func() time.Time {
			return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
		}
		body := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?page=999999&pageSize=1"))
		if body.Pagination != (apiPagination{Page: 999999, PageSize: 1, TotalItems: 2, TotalPages: 2}) {
			t.Fatalf("unexpected out-of-range pagination: %#v", body.Pagination)
		}
		if body.Reviews == nil || len(body.Reviews) != 0 {
			t.Fatalf("out-of-range page must be an empty list: %#v", body.Reviews)
		}
	})
}

func TestReviewsAPIRejectsInvalidQueryParameters(t *testing.T) {
	api := NewAPI(apiTestPoller())
	tests := []struct {
		query string
		code  string
	}{
		{"hours=0", "invalid_hours"},
		{"hours=721", "invalid_hours"},
		{"hours=1.5", "invalid_hours"},
		{"hours=nope", "invalid_hours"},
		{"page=0", "invalid_page"},
		{"page=-1", "invalid_page"},
		{"page=1.5", "invalid_page"},
		{"page=nope", "invalid_page"},
		{"page=999999999999999999999999", "invalid_page"},
		{"pageSize=0", "invalid_page_size"},
		{"pageSize=-1", "invalid_page_size"},
		{"pageSize=101", "invalid_page_size"},
		{"pageSize=1.5", "invalid_page_size"},
		{"pageSize=nope", "invalid_page_size"},
		{"scores=0", "invalid_scores"},
		{"scores=6", "invalid_scores"},
		{"scores=nope", "invalid_scores"},
		{"scores=1,,2", "invalid_scores"},
		{"scores=1&scores=2", "invalid_scores"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			response := serveAPI(t, api, http.MethodGet, "/api/reviews?"+tt.query)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("%s returned %d %s", tt.query, response.Code, response.Body.String())
			}
		})
	}

	response := serveAPI(t, api, http.MethodGet, "/api/reviews?hours=720&page=1&pageSize=100&scores=1,5")
	if response.Code != http.StatusOK {
		t.Fatalf("valid boundaries returned %d %s", response.Code, response.Body.String())
	}
	body := decodeReviewsResponse(t, response)
	if body.Window.Hours != 720 || body.Pagination.Page != 1 || body.Pagination.PageSize != 100 {
		t.Fatalf("valid boundaries were not applied: %#v %#v", body.Window, body.Pagination)
	}
}

func TestReviewsAPICoverageReflectsHistoryLimit(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	oldestAvailable := now.Add(-24 * time.Hour)
	detectedAt := now.Add(-time.Hour)
	poller := apiTestPoller(reviewAt("available", oldestAvailable))
	poller.snapshot.Sync.HistoryLimit = &HistoryLimit{
		DetectedAt:        detectedAt,
		OldestAvailableAt: oldestAvailable,
	}
	api := NewAPI(poller)
	api.now = func() time.Time { return now }

	limited := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?hours=48"))
	if limited.Coverage.Complete || limited.Coverage.LimitedBefore == nil || !limited.Coverage.LimitedBefore.Equal(oldestAvailable) {
		t.Fatalf("expected incomplete coverage before %s, got %#v", oldestAvailable, limited.Coverage)
	}

	boundary := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?hours=24"))
	if !boundary.Coverage.Complete || boundary.Coverage.LimitedBefore != nil {
		t.Fatalf("window beginning at oldest available review should be complete: %#v", boundary.Coverage)
	}
}

func TestReviewsAPICoverageReflectsIntersectingHistoryGap(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	gapAfter := now.Add(-72 * time.Hour)
	gapBefore := now.Add(-48 * time.Hour)
	poller := apiTestPoller(reviewAt("newer", gapBefore), reviewAt("older", gapAfter))
	poller.snapshot.Sync.HistoryGap = &HistoryGap{DetectedAt: now.Add(-time.Hour), After: gapAfter, Before: gapBefore}
	api := NewAPI(poller)
	api.now = func() time.Time { return now }

	intersecting := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?hours=72"))
	if intersecting.Coverage.Complete || intersecting.Coverage.LimitedBefore == nil || !intersecting.Coverage.LimitedBefore.Equal(gapBefore) {
		t.Fatalf("window crossing gap reported complete coverage: %#v", intersecting.Coverage)
	}

	justBeforeBoundary := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?hours=49"))
	if justBeforeBoundary.Coverage.Complete || justBeforeBoundary.Coverage.LimitedBefore == nil || !justBeforeBoundary.Coverage.LimitedBefore.Equal(gapBefore) {
		t.Fatalf("window entering gap reported complete coverage: %#v", justBeforeBoundary.Coverage)
	}

	atNewerBoundary := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?hours=48"))
	if !atNewerBoundary.Coverage.Complete || atNewerBoundary.Coverage.LimitedBefore != nil {
		t.Fatalf("window entirely newer than gap reported incomplete coverage: %#v", atNewerBoundary.Coverage)
	}
}

func TestReviewsAPICoverageUsesNewestIncompleteBoundary(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	limitBoundary := now.Add(-20 * 24 * time.Hour)
	gapBefore := now.Add(-5 * 24 * time.Hour)
	poller := apiTestPoller()
	poller.snapshot.Sync.HistoryLimit = &HistoryLimit{DetectedAt: now, OldestAvailableAt: limitBoundary}
	poller.snapshot.Sync.HistoryGap = &HistoryGap{
		DetectedAt: now, After: now.Add(-10 * 24 * time.Hour), Before: gapBefore,
	}
	api := NewAPI(poller)
	api.now = func() time.Time { return now }

	body := decodeReviewsResponse(t, serveAPI(t, api, http.MethodGet, "/api/reviews?hours=720"))
	if body.Coverage.Complete || body.Coverage.LimitedBefore == nil || !body.Coverage.LimitedBefore.Equal(gapBefore) {
		t.Fatalf("coverage did not report newest trustworthy boundary: %#v", body.Coverage)
	}
}

func TestAppAndHealthAPIExposeSyncState(t *testing.T) {
	poller := apiTestPoller(testReview("one", 1))
	message := "upstream unavailable"
	gap := &HistoryGap{
		DetectedAt: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC),
		After:      time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Before:     time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	}
	poller.snapshot.Sync = SyncState{Status: "gap_detected", LastError: &message, HistoryGap: gap}
	api := NewAPI(poller)

	appResponse := serveAPI(t, api, http.MethodGet, "/api/app")
	if appResponse.Code != http.StatusOK || !strings.Contains(appResponse.Body.String(), `"reviewCount":1`) || !strings.Contains(appResponse.Body.String(), `"status":"gap_detected"`) {
		t.Fatalf("unexpected app response: %d %s", appResponse.Code, appResponse.Body.String())
	}
	healthResponse := serveAPI(t, api, http.MethodGet, "/api/health")
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), `"status":"degraded"`) {
		t.Fatalf("unexpected health response: %d %s", healthResponse.Code, healthResponse.Body.String())
	}
}

func TestHealthAPIDegradesForGapDuringCatchUp(t *testing.T) {
	poller := apiTestPoller()
	poller.snapshot.Sync.Status = "catching_up"
	poller.snapshot.Sync.HistoryGap = &HistoryGap{
		DetectedAt: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC),
		After:      time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Before:     time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	}

	response := serveAPI(t, NewAPI(poller), http.MethodGet, "/api/health")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"degraded"`) || !strings.Contains(response.Body.String(), `"status":"catching_up"`) {
		t.Fatalf("catch-up with a durable gap must be degraded: %d %s", response.Code, response.Body.String())
	}
}

func TestAPIMethodErrorsUseConsistentShape(t *testing.T) {
	api := NewAPI(apiTestPoller())
	response := serveAPI(t, api, http.MethodPost, "/api/app")
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || !strings.Contains(response.Body.String(), `"code":"method_not_allowed"`) {
		t.Fatalf("unexpected method response: %d %#v %s", response.Code, response.Header(), response.Body.String())
	}
}

type reviewsAPIResponse struct {
	Window struct {
		Hours int       `json:"hours"`
		From  time.Time `json:"from"`
		To    time.Time `json:"to"`
	} `json:"window"`
	Pagination apiPagination `json:"pagination"`
	Coverage   apiCoverage   `json:"coverage"`
	Reviews    []apiReview   `json:"reviews"`
}

func serveAPI(t *testing.T, api *API, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	api.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	return response
}

func decodeReviewsResponse(t *testing.T, response *httptest.ResponseRecorder) reviewsAPIResponse {
	t.Helper()
	var body reviewsAPIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v\n%s", err, response.Body.String())
	}
	return body
}

func apiTestPoller(reviews ...Review) *Poller {
	app := testApp()
	snapshot := newSnapshot(app)
	snapshot.Sync.Status = "current"
	snapshot.Reviews = append([]Review(nil), reviews...)
	sortReviews(snapshot.Reviews)
	return &Poller{app: app, snapshot: snapshot}
}

func reviewAt(id string, submittedAt time.Time) Review {
	review := testReview(id, 1)
	review.SubmittedAt = submittedAt
	return review
}
