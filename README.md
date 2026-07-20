# Recent iOS App Store reviews viewer

A small Go service polls Apple's public customer-reviews RSS feed, stores the
reviews durably, and exposes recent reviews to a React interface. The submitted
application is intentionally configured for one app (Spotify in the US), while
its feed and storage boundaries keep the app identity explicit so that adding
more apps later does not require rewriting ingestion.

## Prerequisites

- Go 1.25 or newer (CI and development use Go 1.26)
- Node.js 22.12 or newer, with npm (CI uses Node.js 24)
- Python 3 with Playwright and Chromium only when running the browser E2E test

No database, Docker daemon, or third-party process runner is required.

## Quick start

```sh
make setup
make dev
```

Open [http://localhost:5173](http://localhost:5173). Vite serves the React app
and proxies `/api` requests to the Go service at
[http://localhost:8080](http://localhost:8080).

For a production-style build in which Go serves both the API and the compiled
frontend:

```sh
make run
```

Then open [http://localhost:8080](http://localhost:8080). Press Ctrl-C to stop
the development or production processes. Runtime state remains on disk.

## Commands

| Command | Purpose |
| --- | --- |
| `make setup` | Install frontend dependencies. |
| `make dev` | Run the Go and Vite development servers together; stops both if either exits. |
| `make dev-backend` | Run only the Go server on port 8080. |
| `make dev-frontend` | Run only Vite on port 5173. |
| `make test` | Run backend race tests and frontend tests. |
| `make test-frontend-coverage` | Run frontend tests and print a coverage report. |
| `make setup-e2e` | Install the pinned Playwright test dependency and Chromium. |
| `make test-e2e` | Build and test the real Go API and compiled React app in Chromium. |
| `make vet` | Run Go's static checks. |
| `make build` | Build `bin/reviews-viewer` and `web/dist`. |
| `make run` | Build, then run the production-style server. |
| `make clean` | Remove generated binaries, frontend builds, and coverage. |

The individual `test-backend`, `test-frontend`, `build-backend`, and
`build-frontend` targets are also available.

## Configuration

The service reads `config/app.json` at startup:

```json
{
  "key": "spotify-us",
  "name": "Spotify",
  "appId": "324684580",
  "country": "us",
  "pollInterval": "5m",
  "maxPages": 10,
  "dataDir": "../data",
  "listenAddr": ":8080"
}
```

`appId` is the numeric ID from an App Store URL, while `country` selects the
storefront whose reviews are fetched. The first poll starts immediately;
`pollInterval` is the delay from one completed poll to the start of the next.
`maxPages` bounds work against Apple's finite, shifting RSS history. `dataDir`
is created when needed and should live on durable storage. Relative paths are
resolved from the configuration file, so the shipped `../data` remains at the
repository root even when the binary is launched elsewhere. Restart the service
after changing the configuration.

To use a different configuration file, pass it to the server explicitly:

```sh
go run ./cmd/server -config path/to/app.json
```

The compiled frontend defaults to `../web/dist` relative to that config file.
Override it for a separately deployed frontend with `-web-dir path/to/dist`.

## Architecture and data flow

1. The server loads the app configuration and any existing JSON snapshot.
2. It starts the HTTP server from that snapshot, so cached reviews remain
   available even if Apple cannot be reached.
3. A poller requests RSS pages newest-first through a small `FeedClient`
   boundary. It parses and validates entries into the internal review model.
4. Each successfully fetched page is deduplicated by app key and Apple review
   ID, sorted newest-first, and committed together with durable catch-up
   metadata.
5. Every commit writes a temporary snapshot in `data/` and atomically renames
   it into place. In-memory state advances only after that commit succeeds.
6. The API filters stored reviews by time window and selected scores before
   paginating them; React fetches and renders one page at a time.

The service retains fetched reviews rather than deleting everything older than
48 hours. This makes restart recovery reliable and allows the interface's
7-day and 30-day fallback windows without another Apple request.

## API

All API responses are JSON. Errors consistently use
`{"error":{"code":"...","message":"..."}}`.

### `GET /api/app`

Returns the configured app (`key`, `name`, `appId`, and `country`), total stored
review count, and sync information: current status, last attempt, last success,
last error, in-progress catch-up metadata, and durable history-gap or initial
history-limit markers.

### `GET /api/reviews?hours=48&page=1&pageSize=25&scores=4,5`

Returns reviews submitted inside an inclusive time window. `hours` defaults to
48 and accepts whole hours from 1 through 720. `page` defaults to 1, while
`pageSize` defaults to 25 and accepts values from 1 through 100. Positive pages
beyond the current result set return an empty review list with pagination
metadata rather than an error. Optional `scores` accepts a comma-separated
selection from 1 through 5. Omitting it includes every score; providing an
empty value represents an explicit empty selection and returns no reviews.
Invalid scores return an `invalid_scores` error.

The response contains app metadata, generation time, window boundaries,
`pagination` totals, and reviews ordered by submitted time newest-first with
review ID as a stable tie-breaker. Each review includes its content, author,
score, and submitted time (plus its ID and title). `coverage.complete` is false
when the requested window crosses either an initial history boundary or a
durable gap detected during catch-up. `coverage.limitedBefore` identifies the
newest boundary after which the stored history is known to be continuous. That
boundary is either the oldest available review or the newer edge of a gap.

Examples:

```sh
curl http://localhost:8080/api/reviews
curl 'http://localhost:8080/api/reviews?hours=168&page=2&pageSize=25&scores=1,2'
```

### Operational endpoints

- `GET /api/live` returns HTTP 200 while the process can serve requests.
- `GET /api/ready` returns HTTP 503 until at least one poll has succeeded or
  useful cached reviews are available. It remains HTTP 200 after a later Apple
  failure because the cached API is still usable.
- `GET /api/freshness` separates current, updating, stale, and unavailable data
  from completeness. Its `complete` field becomes true only after a successful
  sync with no durable history gap or initial history limit.
- `GET /api/health` remains the backwards-compatible coarse aggregate. It
  returns HTTP 200 with body status `ok` or `degraded`.

A temporary RSS failure does not discard cached reviews. The nested sync state
distinguishes catch-up, upstream errors, and durable history boundaries.

## Persistence, restart, and catch-up behavior

The JSON snapshot in `data/` contains normalized reviews, their stable IDs, and
sync metadata. It is the polling checkpoint as well as the durable review
store. Page numbers are deliberately not checkpoints because Apple's pages
shift whenever new reviews arrive.

After a restart, the service serves the last valid snapshot immediately and
begins catching up from RSS page 1. It scans through `maxPages` unless it first
finds the review ID that was the newest stored checkpoint when that catch-up
began. Empty pages do not stop the scan because Apple can return a populated
page after an empty one.

Every valid fetched page is atomically merged and persisted before the next
page is requested. The snapshot retains the original checkpoint separately
from newly fetched review IDs, so stopping midway does not lose completed page
progress or cause a restart to stop against its own partial results. A retry
starts from page 1 because Apple's page positions shift, but it continues until
it reaches the original checkpoint or exhausts the configured feed boundary.
Fetch failures keep all successfully committed pages and expose an error state;
unsaved review changes are never published. If writing error metadata also
fails, a volatile in-memory error still keeps the API status honest while the
last durable review snapshot remains unchanged.

Each Apple page request makes at most three attempts for temporary transport
failures and HTTP 408, 425, 429, or 5xx responses. Retries use exponential
backoff from 250 milliseconds, cap waits at two seconds, add jitter, honor
short valid `Retry-After` values, and stop immediately when shutdown cancels
the request. Permanent HTTP and feed-validation failures are not retried.

Finding an old ID proves continuity. If a prior checkpoint exists but is no
longer present anywhere in the available feed, the service stores a durable
history-gap marker and exposes `gap_detected`; it does not silently claim that
all reviews were recovered. The reviews still available from Apple are kept,
and normal polling continues. A first import has no prior checkpoint and
therefore does not report a continuity gap. If that first import fills the last
configured page, the service instead stores an initial history-limit marker so
API clients can identify requested windows that extend beyond the oldest
available review.

Snapshot loading and saving share a 64 MiB limit. A result that would cross the
limit is rejected before rename, leaving the last readable snapshot intact and
logging the persistence failure.

The JSON store is a single-process, single-writer design. Do not run two service
instances against the same `dataDir` and app key; use transactional shared
storage before introducing multiple replicas.

## Frontend behavior

The React viewer defaults to the required 48-hour window and also offers 7-day
and 30-day views for low-volume feeds. Reviews show their title, complete
content, author, local submitted time, and an accessible score. The page
shows 25 reviews at a time with URL-backed Previous/Next navigation. It checks
the backend every three seconds while catch-up is active, then returns to a
60-second steady interval. Initial catch-up without cached reviews uses
skeleton cards; existing cards remain visible during background work. The page
also provides URL-backed 1–5 star checkboxes. Rating changes reset pagination,
and the API applies them before calculating result totals and pages. Helpful
empty states distinguish no selected ratings from no matching reviews. The UI
also exposes error, incomplete-coverage, and cached-data-with-sync-warning
states. Its refresh action reads the backend; it does not bypass the poll
schedule to hit Apple directly.

## Dependencies

The backend intentionally uses only Go's standard library: `net/http` for RSS
and API traffic, `encoding/json` for parsing and snapshots, and `log/slog` for
structured diagnostics.

React satisfies the assignment's UI requirement. TypeScript provides useful
API-model checks, and Vite supplies a compact development/build toolchain.
Vitest, Testing Library, and jsdom are development-only dependencies used to
test visible and accessible component behavior without coupling tests to
implementation details. Python Playwright is pinned as a test-only dependency
for the production browser E2E path; it is not required to build or run the
application. There is no UI component library, router, state library, or
backend framework.

## Testing

```sh
make test
make test-frontend-coverage
make vet
make build
make setup-e2e
make test-e2e
```

Backend tests cover feed parsing, bounded transient retries and backoff,
pagination and original-checkpoint overlap, deduplication, validation, atomic
per-page persistence, snapshot-size symmetry, corrupt snapshots, API
pagination/filtering, error responses and coverage boundaries, interrupted and
restarted catch-up, initial history limits, durable gap detection, and
reconstructing the service from the same data directory. Frontend tests cover
required review fields, URL-backed pagination, rating filters and time windows,
adaptive polling, focus management, refresh behavior, coverage warnings, and
loading, empty, error, and stale states. CI repeats Go tests with race
detection, `go vet`, frontend tests, both production builds, and a headless
Chromium E2E test. The browser test seeds an isolated temporary JSON snapshot,
starts the compiled Go server serving `web/dist`, verifies the operational and
reviews APIs plus required rendered fields, exercises newest-first pagination,
browser history, the 7-day window, and rating filtering, and redirects the
poller's Apple request to a closed local proxy so it is deterministic and
offline-safe.

For a manual restart check, run the service until at least one poll succeeds,
stop it, and start it again with the same `dataDir`. The cached reviews should
be available before the new Apple poll completes.

## Limits and future multi-app support

Apple's RSS feed exposes only a finite amount of recent history. If more reviews
arrive while this service is offline than the available pages retain, the
missing reviews cannot be recovered from this source. The durable gap marker
or initial history-limit marker makes that uncertainty visible; a shorter
backend polling interval lowers its likelihood.
The feed exposes the review timestamp as `updated`; the service treats that
value as the submitted time displayed and filtered by the API.

The API intentionally keeps offset pagination for this submission. It preserves
simple numbered Previous/Next navigation and useful totals, while occasional
page movement is acceptable for a live monitoring view. New reviews inserted
between page requests can still shift an offset boundary and cause a duplicate
or skipped card. If stable traversal becomes a product requirement, the next
step is an opaque cursor built from the `(submittedAt, reviewID)` sort key and
bound to the active time and score filters; it can be added alongside the
current page parameters for backward compatibility.

The current API and interface intentionally expose one configured app. Internally,
the feed client accepts app configuration, every review carries an app key, and
the store loads and saves snapshots by app key. Multi-app support can therefore
be added by loading a configuration collection, constructing one poller per
app, and adding app-list/scoped-review routes plus a UI selector. The parser,
review model, snapshot format, and store implementation do not need to change.
