package reviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxFeedResponseBytes = 4 << 20

const (
	defaultMaxFeedAttempts = 3
	defaultBaseRetryDelay  = 250 * time.Millisecond
	defaultMaxRetryDelay   = 2 * time.Second
)

// FeedPage contains one normalized page returned by a review feed.
type FeedPage struct {
	Reviews []Review
}

// FeedClient fetches review pages for the poller.
type FeedClient interface {
	FetchPage(ctx context.Context, app AppConfig, page int) (FeedPage, error)
}

// AppleFeedClient fetches Apple's public customer-review RSS feed.
type AppleFeedClient struct {
	client         *http.Client
	baseURL        string
	now            func() time.Time
	maxAttempts    int
	baseRetryDelay time.Duration
	maxRetryDelay  time.Duration
	jitter         func(time.Duration) time.Duration
	sleep          func(context.Context, time.Duration) error
}

// NewAppleFeedClient creates an Apple feed client using client for requests.
func NewAppleFeedClient(client *http.Client) *AppleFeedClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &AppleFeedClient{
		client: client, baseURL: "https://itunes.apple.com", now: time.Now,
		maxAttempts: defaultMaxFeedAttempts, baseRetryDelay: defaultBaseRetryDelay,
		maxRetryDelay: defaultMaxRetryDelay, jitter: defaultRetryJitter, sleep: sleepContext,
	}
}

// FetchPage fetches and validates one page of reviews for app.
func (c *AppleFeedClient) FetchPage(ctx context.Context, app AppConfig, page int) (FeedPage, error) {
	if page < 1 || page > 10 {
		return FeedPage{}, fmt.Errorf("page must be between 1 and 10")
	}
	attempts := c.maxAttempts
	if attempts < 1 {
		attempts = defaultMaxFeedAttempts
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		result, transient, retryAfter, err := c.fetchPageOnce(ctx, app, page)
		if err == nil {
			return result, nil
		}
		if !transient || attempt == attempts {
			return FeedPage{}, err
		}
		delay := c.retryDelay(attempt, retryAfter)
		sleep := c.sleep
		if sleep == nil {
			sleep = sleepContext
		}
		if err := sleep(ctx, delay); err != nil {
			return FeedPage{}, fmt.Errorf("wait to retry feed page %d: %w", page, err)
		}
	}
	panic("unreachable")
}

func (c *AppleFeedClient) fetchPageOnce(ctx context.Context, app AppConfig, page int) (FeedPage, bool, string, error) {
	endpoint := fmt.Sprintf("%s/%s/rss/customerreviews/id=%s/sortBy=mostRecent/page=%d/json",
		strings.TrimRight(c.baseURL, "/"), url.PathEscape(app.Country), url.PathEscape(app.AppID), page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return FeedPage{}, false, "", fmt.Errorf("create feed request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "recent-ios-reviews/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		// A client-owned timeout also wraps context.DeadlineExceeded, but the
		// caller's context remains active and a fresh attempt can succeed. Caller
		// cancellation is terminal; so is an explicit canceled transport error.
		transient := ctx.Err() == nil && !errors.Is(err, context.Canceled)
		return FeedPage{}, transient, "", fmt.Errorf("fetch feed page %d: %w", page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return FeedPage{}, isTransientFeedStatus(resp.StatusCode), resp.Header.Get("Retry-After"), fmt.Errorf("fetch feed page %d: unexpected HTTP status %d", page, resp.StatusCode)
	}

	reviews, err := parseFeed(io.LimitReader(resp.Body, maxFeedResponseBytes+1), app, c.now().UTC())
	if err != nil {
		return FeedPage{}, false, "", fmt.Errorf("parse feed page %d: %w", page, err)
	}
	return FeedPage{Reviews: reviews}, false, "", nil
}

func (c *AppleFeedClient) retryDelay(failedAttempt int, retryAfter string) time.Duration {
	maxDelay := c.maxRetryDelay
	if maxDelay <= 0 {
		maxDelay = defaultMaxRetryDelay
	}
	if delay, ok := parseRetryAfter(retryAfter, c.now(), maxDelay); ok {
		return delay
	}
	baseDelay := c.baseRetryDelay
	if baseDelay <= 0 {
		baseDelay = defaultBaseRetryDelay
	}
	delay := baseDelay
	for retry := 1; retry < failedAttempt && delay < maxDelay; retry++ {
		if delay > maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	jitter := c.jitter
	if jitter == nil {
		jitter = defaultRetryJitter
	}
	delay = jitter(delay)
	if delay < 0 {
		return 0
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func isTransientFeedStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || (status >= 500 && status <= 599)
}

func parseRetryAfter(value string, now time.Time, maxDelay time.Duration) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		return delay, delay <= maxDelay
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		delay := retryAt.Sub(now)
		return delay, delay >= 0 && delay <= maxDelay
	}
	return 0, false
}

func defaultRetryJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	halfRange := delay / 2
	return delay*3/4 + time.Duration(rand.Int63n(int64(halfRange)+1))
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type appleFeed struct {
	Feed *struct {
		Entries []struct {
			ID      appleLabel `json:"id"`
			Title   appleLabel `json:"title"`
			Content appleLabel `json:"content"`
			Updated appleLabel `json:"updated"`
			Rating  appleLabel `json:"im:rating"`
			Author  struct {
				Name appleLabel `json:"name"`
			} `json:"author"`
		} `json:"entry"`
	} `json:"feed"`
}

type appleLabel struct {
	Label string `json:"label"`
}

func parseFeed(r io.Reader, app AppConfig, fetchedAt time.Time) ([]Review, error) {
	var payload appleFeed
	dec := json.NewDecoder(r)
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, err
	}
	if payload.Feed == nil {
		return nil, errors.New("response has no feed object")
	}

	reviews := make([]Review, 0, len(payload.Feed.Entries))
	seen := make(map[string]struct{}, len(payload.Feed.Entries))
	for i, entry := range payload.Feed.Entries {
		id := strings.TrimSpace(entry.ID.Label)
		if id == "" {
			return nil, fmt.Errorf("entry %d has no id", i)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("entry %d repeats id %q", i, id)
		}
		seen[id] = struct{}{}
		score, err := strconv.Atoi(entry.Rating.Label)
		if err != nil || score < 1 || score > 5 {
			return nil, fmt.Errorf("entry %q has invalid rating %q", id, entry.Rating.Label)
		}
		submittedAt, err := time.Parse(time.RFC3339, entry.Updated.Label)
		if err != nil {
			return nil, fmt.Errorf("entry %q has invalid updated timestamp %q", id, entry.Updated.Label)
		}
		author := strings.TrimSpace(entry.Author.Name.Label)
		if author == "" {
			return nil, fmt.Errorf("entry %q has no author", id)
		}
		reviews = append(reviews, Review{
			ID: id, AppKey: app.Key, Title: entry.Title.Label, Content: entry.Content.Label,
			Author: author, Score: score, SubmittedAt: submittedAt.UTC(), FetchedAt: fetchedAt,
		})
	}
	return reviews, nil
}
