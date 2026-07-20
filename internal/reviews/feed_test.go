package reviews

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseFeed(t *testing.T) {
	fetchedAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	fixture := `{"feed":{"entry":[{
        "id":{"label":"123"},
        "title":{"label":"Great"},
        "content":{"label":"Works well"},
        "updated":{"label":"2026-07-17T08:30:00-07:00"},
        "im:rating":{"label":"5"},
        "author":{"name":{"label":"Ada"}}
    }]}}`

	got, err := parseFeed(strings.NewReader(fixture), testApp(), fetchedAt)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d reviews, want 1", len(got))
	}
	review := got[0]
	if review.ID != "123" || review.AppKey != "spotify-us" || review.Title != "Great" || review.Content != "Works well" || review.Author != "Ada" || review.Score != 5 {
		t.Fatalf("unexpected review: %#v", review)
	}
	wantSubmitted := time.Date(2026, 7, 17, 15, 30, 0, 0, time.UTC)
	if !review.SubmittedAt.Equal(wantSubmitted) || !review.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("unexpected timestamps: submitted=%v fetched=%v", review.SubmittedAt, review.FetchedAt)
	}
}

func TestParseFeedRejectsMissingFeedObject(t *testing.T) {
	for _, fixture := range []string{`{}`, `{"feed":null}`} {
		if _, err := parseFeed(strings.NewReader(fixture), testApp(), time.Now()); err == nil || !strings.Contains(err.Error(), "no feed object") {
			t.Fatalf("parseFeed(%s) returned error %v, want missing feed object", fixture, err)
		}
	}
}

func TestParseFeedAllowsEmptyFeed(t *testing.T) {
	for _, fixture := range []string{`{"feed":{}}`, `{"feed":{"entry":[]}}`} {
		got, err := parseFeed(strings.NewReader(fixture), testApp(), time.Now())
		if err != nil {
			t.Fatalf("parseFeed(%s) returned error: %v", fixture, err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("parseFeed(%s) returned %#v, want empty review list", fixture, got)
		}
	}
}

func TestParseFeedRejectsInvalidEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   string
		message string
	}{
		{"missing id", `"id":{"label":""},"im:rating":{"label":"5"},"updated":{"label":"2026-07-17T00:00:00Z"},"author":{"name":{"label":"A"}}`, "no id"},
		{"invalid score", `"id":{"label":"1"},"im:rating":{"label":"6"},"updated":{"label":"2026-07-17T00:00:00Z"},"author":{"name":{"label":"A"}}`, "invalid rating"},
		{"invalid timestamp", `"id":{"label":"1"},"im:rating":{"label":"5"},"updated":{"label":"yesterday"},"author":{"name":{"label":"A"}}`, "invalid updated timestamp"},
		{"missing author", `"id":{"label":"1"},"im:rating":{"label":"5"},"updated":{"label":"2026-07-17T00:00:00Z"},"author":{"name":{"label":""}}`, "no author"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := fmt.Sprintf(`{"feed":{"entry":[{%s}]}}`, tc.entry)
			_, err := parseFeed(strings.NewReader(fixture), testApp(), time.Now())
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got error %v, want containing %q", err, tc.message)
			}
		})
	}
}

func TestParseFeedRejectsMalformedJSONAndDuplicateIDs(t *testing.T) {
	if _, err := parseFeed(strings.NewReader(`{"feed":`), testApp(), time.Now()); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
	fixture := `{"feed":{"entry":[
      {"id":{"label":"1"},"im:rating":{"label":"5"},"updated":{"label":"2026-07-17T00:00:00Z"},"author":{"name":{"label":"A"}}},
      {"id":{"label":"1"},"im:rating":{"label":"4"},"updated":{"label":"2026-07-16T00:00:00Z"},"author":{"name":{"label":"B"}}}
    ]}}`
	if _, err := parseFeed(strings.NewReader(fixture), testApp(), time.Now()); err == nil || !strings.Contains(err.Error(), "repeats id") {
		t.Fatalf("duplicate IDs returned error %v", err)
	}
}

func TestAppleFeedClientHTTPBehavior(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"feed":{"entry":[]}}`))
	}))
	defer server.Close()

	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	page, err := client.FetchPage(context.Background(), testApp(), 3)
	if err != nil {
		t.Fatalf("FetchPage returned error: %v", err)
	}
	if len(page.Reviews) != 0 || path != "/us/rss/customerreviews/id=324684580/sortBy=mostRecent/page=3/json" {
		t.Fatalf("unexpected response/path: %#v %q", page, path)
	}

	if _, err := client.FetchPage(context.Background(), testApp(), 11); err == nil {
		t.Fatal("invalid page was accepted")
	}
}

func TestAppleFeedClientDoesNotRetryPermanentHTTPError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	client.sleep = func(context.Context, time.Duration) error {
		t.Fatal("permanent status attempted a retry")
		return nil
	}
	_, err := client.FetchPage(context.Background(), testApp(), 1)
	if err == nil || !strings.Contains(err.Error(), "404") || calls.Load() != 1 {
		t.Fatalf("got error %v, want HTTP status", err)
	}
}

func TestAppleFeedClientRetriesTransientStatusWithBackoffAndJitter(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"feed":{"entry":[]}}`))
	}))
	defer server.Close()

	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	client.baseRetryDelay = 100 * time.Millisecond
	client.maxRetryDelay = time.Second
	client.jitter = func(delay time.Duration) time.Duration { return delay + 25*time.Millisecond }
	var delays []time.Duration
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	if _, err := client.FetchPage(context.Background(), testApp(), 1); err != nil {
		t.Fatalf("FetchPage returned error after transient retries: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("made %d attempts, want 3", calls.Load())
	}
	want := []time.Duration{125 * time.Millisecond, 225 * time.Millisecond}
	if len(delays) != len(want) || delays[0] != want[0] || delays[1] != want[1] {
		t.Fatalf("retry delays %v, want %v", delays, want)
	}
}

func TestAppleFeedClientCapsAttemptsAndRetryDelay(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	client.maxAttempts = 2
	client.baseRetryDelay = 200 * time.Millisecond
	client.maxRetryDelay = 150 * time.Millisecond
	client.jitter = func(delay time.Duration) time.Duration { return delay + time.Second }
	var delays []time.Duration
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	_, err := client.FetchPage(context.Background(), testApp(), 1)
	if err == nil || calls.Load() != 2 || len(delays) != 1 || delays[0] != 150*time.Millisecond {
		t.Fatalf("caps produced error=%v calls=%d delays=%v", err, calls.Load(), delays)
	}
}

func TestTransientFeedStatusClassification(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests, 500, 599} {
		if !isTransientFeedStatus(status) {
			t.Errorf("status %d should be transient", status)
		}
	}
	for _, status := range []int{400, 404, 499, 600} {
		if isTransientFeedStatus(status) {
			t.Errorf("status %d should be permanent", status)
		}
	}
}

func TestAppleFeedClientHonorsSmallRetryAfter(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"feed":{"entry":[]}}`))
	}))
	defer server.Close()

	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	client.jitter = func(time.Duration) time.Duration { return 0 }
	var delay time.Duration
	client.sleep = func(_ context.Context, got time.Duration) error {
		delay = got
		return nil
	}
	if _, err := client.FetchPage(context.Background(), testApp(), 1); err != nil {
		t.Fatalf("FetchPage returned error: %v", err)
	}
	if calls.Load() != 2 || delay != time.Second {
		t.Fatalf("Retry-After produced calls=%d delay=%s, want 2 and 1s", calls.Load(), delay)
	}
}

func TestAppleFeedClientRetriesTransientTransportError(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("connection reset")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"feed":{"entry":[]}}`)),
		}, nil
	})
	client := NewAppleFeedClient(&http.Client{Transport: transport})
	client.jitter = func(delay time.Duration) time.Duration { return delay }
	client.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.FetchPage(context.Background(), testApp(), 1); err != nil {
		t.Fatalf("FetchPage returned error after transport retry: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("transport called %d times, want 2", calls.Load())
	}
}

func TestAppleFeedClientRetriesClientTimeoutWithActiveCallerContext(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"feed":{"entry":[]}}`)),
		}, nil
	})
	httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Millisecond}
	client := NewAppleFeedClient(httpClient)
	client.jitter = func(delay time.Duration) time.Duration { return delay }
	client.sleep = func(context.Context, time.Duration) error { return nil }

	callerCtx := context.Background()
	if _, err := client.FetchPage(callerCtx, testApp(), 1); err != nil {
		t.Fatalf("FetchPage did not recover from client-owned timeout: %v", err)
	}
	if callerCtx.Err() != nil || calls.Load() != 2 {
		t.Fatalf("caller context error=%v transport calls=%d, want active context and 2 calls", callerCtx.Err(), calls.Load())
	}
}

func TestAppleFeedClientDoesNotRetryCanceledTransport(t *testing.T) {
	var calls atomic.Int32
	client := NewAppleFeedClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, context.Canceled
	})})
	_, err := client.FetchPage(context.Background(), testApp(), 1)
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatalf("canceled transport returned error=%v calls=%d", err, calls.Load())
	}
}

func TestAppleFeedClientCancellationStopsRetryWait(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	client.sleep = func(ctx context.Context, delay time.Duration) error {
		cancel()
		return sleepContext(ctx, delay)
	}
	_, err := client.FetchPage(ctx, testApp(), 1)
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatalf("canceled retry returned error=%v calls=%d", err, calls.Load())
	}
}

func TestAppleFeedClientDoesNotRetryParseErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"feed":`))
	}))
	defer server.Close()
	client := NewAppleFeedClient(server.Client())
	client.baseURL = server.URL
	_, err := client.FetchPage(context.Background(), testApp(), 1)
	if err == nil || !strings.Contains(err.Error(), "parse feed page") || calls.Load() != 1 {
		t.Fatalf("parse error returned error=%v calls=%d", err, calls.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testApp() AppConfig {
	return AppConfig{
		Key: "spotify-us", Name: "Spotify", AppID: "324684580", Country: "us",
		PollInterval: 5 * time.Minute, MaxPages: 10, DataDir: "data", ListenAddr: ":8080",
	}
}
