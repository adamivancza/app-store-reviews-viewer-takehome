package reviews

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// API exposes the current review snapshot over HTTP.
type API struct {
	poller *Poller
	now    func() time.Time
}

// NewAPI creates an HTTP API backed by poller.
func NewAPI(poller *Poller) *API { return &API{poller: poller, now: time.Now} }

// Register adds all API routes to mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/app", a.app)
	mux.HandleFunc("/api/reviews", a.reviews)
	mux.HandleFunc("/api/live", a.live)
	mux.HandleFunc("/api/ready", a.ready)
	mux.HandleFunc("/api/freshness", a.freshness)
	mux.HandleFunc("/api/health", a.health)
}

func (a *API) app(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	snapshot := a.poller.Snapshot()
	writeJSON(w, http.StatusOK, struct {
		App         AppConfig `json:"app"`
		ReviewCount int       `json:"reviewCount"`
		Sync        SyncState `json:"sync"`
	}{publicApp(snapshot.App), len(snapshot.Reviews), snapshot.Sync})
}

type apiReview struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      string    `json:"author"`
	Score       int       `json:"score"`
	SubmittedAt time.Time `json:"submittedAt"`
}

type apiPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

type apiCoverage struct {
	Complete      bool       `json:"complete"`
	LimitedBefore *time.Time `json:"limitedBefore"`
}

type reviewQuery struct {
	hours          int
	page           int
	pageSize       int
	scores         map[int]struct{}
	scoresProvided bool
}

type apiProblem struct {
	code    string
	message string
}

type reviewsResponse struct {
	App         AppConfig `json:"app"`
	GeneratedAt time.Time `json:"generatedAt"`
	Window      struct {
		Hours int       `json:"hours"`
		From  time.Time `json:"from"`
		To    time.Time `json:"to"`
	} `json:"window"`
	Pagination apiPagination `json:"pagination"`
	Coverage   apiCoverage   `json:"coverage"`
	Reviews    []apiReview   `json:"reviews"`
}

func (a *API) reviews(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	query, problem := parseReviewQuery(r.URL.Query())
	if problem != nil {
		writeAPIError(w, http.StatusBadRequest, problem.code, problem.message)
		return
	}
	writeJSON(w, http.StatusOK, buildReviewsResponse(a.poller.Snapshot(), query, a.now().UTC()))
}

func parseReviewQuery(values url.Values) (reviewQuery, *apiProblem) {
	query := reviewQuery{hours: 48, page: 1, pageSize: 25}
	if raw := values.Get("hours"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 720 {
			return reviewQuery{}, &apiProblem{code: "invalid_hours", message: "hours must be an integer between 1 and 720"}
		}
		query.hours = value
	}
	if raw := values.Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return reviewQuery{}, &apiProblem{code: "invalid_page", message: "page must be a positive integer"}
		}
		query.page = value
	}
	if raw := values.Get("pageSize"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return reviewQuery{}, &apiProblem{code: "invalid_page_size", message: "pageSize must be an integer between 1 and 100"}
		}
		query.pageSize = value
	}
	scores, scoresProvided, err := parseScores(values)
	if err != nil {
		return reviewQuery{}, &apiProblem{code: "invalid_scores", message: "scores must be a comma-separated list containing only integers from 1 to 5"}
	}
	query.scores = scores
	query.scoresProvided = scoresProvided
	return query, nil
}

func buildReviewsResponse(snapshot Snapshot, query reviewQuery, now time.Time) reviewsResponse {
	from := now.Add(-time.Duration(query.hours) * time.Hour)
	items := make([]apiReview, 0)
	for _, review := range snapshot.Reviews {
		if review.SubmittedAt.Before(from) || review.SubmittedAt.After(now) {
			continue
		}
		if query.scoresProvided {
			if _, selected := query.scores[review.Score]; !selected {
				continue
			}
		}
		items = append(items, apiReview{
			ID: review.ID, Title: review.Title, Content: review.Content, Author: review.Author,
			Score: review.Score, SubmittedAt: review.SubmittedAt,
		})
	}

	totalItems := len(items)
	totalPages := 0
	if totalItems > 0 {
		totalPages = 1 + (totalItems-1)/query.pageSize
	}
	pagedItems := make([]apiReview, 0)
	if query.page <= totalPages {
		start := (query.page - 1) * query.pageSize
		end := min(start+query.pageSize, totalItems)
		pagedItems = items[start:end]
	}

	coverage := apiCoverage{Complete: true}
	if limit := snapshot.Sync.HistoryLimit; limit != nil && from.Before(limit.OldestAvailableAt) {
		markCoverageLimited(&coverage, limit.OldestAvailableAt)
	}
	if gap := snapshot.Sync.HistoryGap; gap != nil {
		gapAfter, gapBefore := gap.After, gap.Before
		if gapBefore.Before(gapAfter) {
			gapAfter, gapBefore = gapBefore, gapAfter
		}
		// The missing interval is between the last known older review and the
		// first known newer review. Touching either boundary alone is complete.
		if from.Before(gapBefore) && now.After(gapAfter) {
			markCoverageLimited(&coverage, gapBefore)
		}
	}

	return reviewsResponse{
		App: publicApp(snapshot.App), GeneratedAt: now,
		Window: struct {
			Hours int       `json:"hours"`
			From  time.Time `json:"from"`
			To    time.Time `json:"to"`
		}{query.hours, from, now},
		Pagination: apiPagination{
			Page: query.page, PageSize: query.pageSize, TotalItems: totalItems, TotalPages: totalPages,
		},
		Coverage: coverage,
		Reviews:  pagedItems,
	}
}

func markCoverageLimited(coverage *apiCoverage, boundary time.Time) {
	coverage.Complete = false
	if coverage.LimitedBefore == nil || boundary.After(*coverage.LimitedBefore) {
		limitedBefore := boundary
		coverage.LimitedBefore = &limitedBefore
	}
}

func parseScores(values url.Values) (map[int]struct{}, bool, error) {
	rawValues, provided := values["scores"]
	if !provided {
		return nil, false, nil
	}
	if len(rawValues) != 1 {
		return nil, true, errors.New("scores must be provided once")
	}
	selected := make(map[int]struct{})
	if rawValues[0] == "" {
		return selected, true, nil
	}
	for _, raw := range strings.Split(rawValues[0], ",") {
		score, err := strconv.Atoi(raw)
		if err != nil || score < 1 || score > 5 {
			return nil, true, errors.New("invalid score")
		}
		selected[score] = struct{}{}
	}
	return selected, true, nil
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	snapshot := a.poller.Snapshot()
	status := "ok"
	if snapshot.Sync.Status == SyncStatusError || snapshot.Sync.Status == SyncStatusGap || snapshot.Sync.HistoryGap != nil {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, struct {
		Status string    `json:"status"`
		App    AppConfig `json:"app"`
		Sync   SyncState `json:"sync"`
	}{status, publicApp(snapshot.App), snapshot.Sync})
}

func (a *API) live(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	snapshot := a.poller.Snapshot()
	ready := snapshot.Sync.LastSuccessAt != nil || len(snapshot.Reviews) > 0
	status := "ready"
	statusCode := http.StatusOK
	if !ready {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(w, statusCode, struct {
		Status string    `json:"status"`
		App    AppConfig `json:"app"`
	}{status, publicApp(snapshot.App)})
}

func (a *API) freshness(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	snapshot := a.poller.Snapshot()
	status := "current"
	switch snapshot.Sync.Status {
	case SyncStatusCatchingUp:
		status = "updating"
	case SyncStatusError:
		if snapshot.Sync.LastSuccessAt == nil && len(snapshot.Reviews) == 0 {
			status = "unavailable"
		} else {
			status = "stale"
		}
	}
	complete := snapshot.Sync.LastSuccessAt != nil && snapshot.Sync.HistoryGap == nil && snapshot.Sync.HistoryLimit == nil
	writeJSON(w, http.StatusOK, struct {
		Status   string    `json:"status"`
		Complete bool      `json:"complete"`
		App      AppConfig `json:"app"`
		Sync     SyncState `json:"sync"`
	}{status, complete, publicApp(snapshot.App), snapshot.Sync})
}

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
	return false
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{code, message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
