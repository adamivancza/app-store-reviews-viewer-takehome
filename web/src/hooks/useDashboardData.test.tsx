import { act, renderHook, waitFor } from "@testing-library/react";
import { getApp, getReviews } from "../api/reviews";
import { appResponse, reviewsResponse, view } from "../test/fixtures";
import type { DashboardNavigation } from "./types";
import { useDashboardData } from "./useDashboardData";

vi.mock("../api/reviews", () => ({ getApp: vi.fn(), getReviews: vi.fn() }));

const mockedGetApp = vi.mocked(getApp);
const mockedGetReviews = vi.mocked(getReviews);

function navigation(overrides: Partial<DashboardNavigation> = {}): DashboardNavigation {
  return {
    view: view(),
    feedRef: { current: null },
    isPageChanging: false,
    isFilterChanging: false,
    selectHours: vi.fn(),
    selectPage: vi.fn(),
    toggleScore: vi.fn(),
    handleSkipLink: vi.fn(),
    consumeSkippedViewLoad: vi.fn(() => false),
    canonicalizePage: vi.fn(),
    handleSuccessfulLoad: vi.fn(),
    handleNavigationError: vi.fn(),
    finishLoad: vi.fn(),
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useDashboardData", () => {
  beforeEach(() => {
    mockedGetApp.mockResolvedValue(appResponse());
    mockedGetReviews.mockResolvedValue(reviewsResponse());
  });

  it("loads the app and current filtered view on first render", async () => {
    const nav = navigation({ view: view({ scores: [5] }) });
    const { result } = renderHook(() => useDashboardData(nav));
    await waitFor(() => expect(result.current.appData?.app.name).toBe("Spotify"));
    expect(mockedGetReviews).toHaveBeenCalledWith(48, 1, [5], expect.any(AbortSignal));
    expect(nav.handleSuccessfulLoad).toHaveBeenCalledWith(nav.view);
    expect(nav.finishLoad).toHaveBeenCalled();
  });

  it("retains cached reviews and their original window while a new view is loading", async () => {
    const nav = navigation();
    const { result, rerender } = renderHook(({ current }) => useDashboardData(current), { initialProps: { current: nav } });
    await waitFor(() => expect(result.current.reviewsData).not.toBeNull());
    const delayedReviews = deferred<ReturnType<typeof reviewsResponse>>();
    mockedGetReviews.mockReturnValueOnce(delayedReviews.promise);
    const nextNav = navigation({ view: view({ hours: 168 }) });
    rerender({ current: nextNav });
    expect(result.current.reviewsData?.window.hours).toBe(48);
    await act(async () => delayedReviews.resolve(reviewsResponse({ window: { hours: 168, from: "", to: "" } })));
    await waitFor(() => expect(result.current.reviewsData?.window.hours).toBe(168));
  });

  it("shows a spinner only for a manual refresh", async () => {
    const nav = navigation();
    const { result } = renderHook(() => useDashboardData(nav));
    await waitFor(() => expect(result.current.reviewsData).not.toBeNull());
    const nextApp = deferred<ReturnType<typeof appResponse>>();
    mockedGetApp.mockReturnValueOnce(nextApp.promise);
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.loadDashboard("manual");
    });
    expect(result.current.isRefreshing).toBe(true);
    expect(result.current.isLoading).toBe(false);
    await act(async () => nextApp.resolve(appResponse()));
    expect(result.current.isRefreshing).toBe(true);
    await act(async () => pending);
    expect(result.current.isRefreshing).toBe(false);
  });

  it("keeps cached reviews visible when a manual refresh fails", async () => {
    const nav = navigation();
    const { result } = renderHook(() => useDashboardData(nav));
    await waitFor(() => expect(result.current.reviewsData).not.toBeNull());
    mockedGetApp.mockRejectedValueOnce(new Error("offline"));
    await act(async () => result.current.loadDashboard("manual"));
    expect(result.current.reviewsData?.reviews[0]?.id).toBe("review-1");
    expect(result.current.requestError).toBe("offline");
  });

  it("rolls back a failed navigation through its navigation collaborator", async () => {
    const nav = navigation();
    const { result } = renderHook(() => useDashboardData(nav));
    await waitFor(() => expect(result.current.reviewsData).not.toBeNull());
    mockedGetApp.mockRejectedValueOnce(new Error("offline"));
    await act(async () => result.current.loadDashboard("navigation"));
    expect(nav.handleNavigationError).toHaveBeenCalledWith(nav.view);
  });

  it("canonicalizes a page beyond the final page instead of displaying it", async () => {
    const requested = view({ page: 9 });
    const nav = navigation({ view: requested });
    mockedGetReviews.mockResolvedValue(reviewsResponse({ pagination: { page: 9, pageSize: 25, totalItems: 51, totalPages: 3 } }));
    renderHook(() => useDashboardData(nav));
    await waitFor(() => expect(nav.canonicalizePage).toHaveBeenCalledWith(requested, 3));
    expect(nav.handleSuccessfulLoad).not.toHaveBeenCalled();
  });
});
