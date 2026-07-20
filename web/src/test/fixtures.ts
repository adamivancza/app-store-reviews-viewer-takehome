import type {
  AppResponse,
  DashboardLoadingState,
  ReviewsResponse,
  ViewState,
} from "../shared/reviews/types";

export const view = (overrides: Partial<ViewState> = {}): ViewState => ({
  hours: 48,
  page: 1,
  scores: [5, 4, 3, 2, 1],
  ...overrides,
});

export const appResponse = (overrides: Partial<AppResponse> = {}): AppResponse => ({
  app: { key: "spotify", name: "Spotify", appId: "324684580", country: "us" },
  reviewCount: 2,
  sync: {
    status: "current",
    lastAttemptAt: "2026-07-17T08:00:00Z",
    lastSuccessAt: "2026-07-17T08:00:00Z",
    lastError: null,
    historyGap: null,
  },
  ...overrides,
});

export const reviewsResponse = (
  overrides: Partial<ReviewsResponse> = {},
): ReviewsResponse => ({
  app: appResponse().app,
  generatedAt: "2026-07-17T08:03:00Z",
  window: { hours: 48, from: "2026-07-15T08:03:00Z", to: "2026-07-17T08:03:00Z" },
  pagination: { page: 1, pageSize: 25, totalItems: 2, totalPages: 1 },
  coverage: { complete: true, limitedBefore: null },
  reviews: [
    {
      id: "review-1",
      title: "Great playlists",
      content: "The recommendations have been excellent.",
      author: "Ada",
      score: 5,
      submittedAt: "2026-07-17T07:00:00Z",
    },
  ],
  ...overrides,
});

export const loading = (
  overrides: Partial<DashboardLoadingState> = {},
): DashboardLoadingState => ({
  initial: false,
  refreshing: false,
  pageChanging: false,
  filterChanging: false,
  ...overrides,
});

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
