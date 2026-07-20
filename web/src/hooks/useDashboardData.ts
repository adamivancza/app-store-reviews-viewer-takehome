import { useCallback, useEffect, useRef, useState } from "react";
import { getApp, getReviews } from "../api/reviews";
import type { AppResponse, ReviewsResponse } from "../shared/reviews/types";
import { DEFAULT_PAGE, MIN_REFRESH_FEEDBACK_MS } from "./constants";
import type { DashboardData, DashboardNavigation, LoadMode } from "./types";

export function useDashboardData(
  navigation: DashboardNavigation,
): DashboardData {
  const {
    view,
    canonicalizePage,
    consumeSkippedViewLoad,
    finishLoad,
    handleNavigationError,
    handleSuccessfulLoad,
  } = navigation;
  const [appData, setAppData] = useState<AppResponse | null>(null);
  const [reviewsData, setReviewsData] = useState<ReviewsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);
  const requestSequence = useRef(0);

  const loadDashboard = useCallback(
    async (mode: LoadMode, signal?: AbortSignal) => {
      const requestId = ++requestSequence.current;
      const startedAt = Date.now();
      const requestedView = {
        ...view,
        scores: [...view.scores],
      };
      let canonicalizedPage = false;
      if (mode === "initial") setIsLoading(true);
      if (mode === "manual") setIsRefreshing(true);
      try {
        const [nextApp, nextReviews] = await Promise.all([
          getApp(signal),
          getReviews(
            requestedView.hours,
            requestedView.page,
            requestedView.scores,
            signal,
          ),
        ]);
        if (requestId !== requestSequence.current) return;
        const lastPage = Math.max(
          DEFAULT_PAGE,
          nextReviews.pagination.totalPages,
        );
        if (requestedView.page > lastPage) {
          canonicalizedPage = true;
          canonicalizePage(requestedView, lastPage);
          return;
        }
        setAppData(nextApp);
        setReviewsData(nextReviews);
        setRequestError(null);
        handleSuccessfulLoad(requestedView);
      } catch (error) {
        if (signal?.aborted || requestId !== requestSequence.current) return;
        setRequestError(
          error instanceof Error ? error.message : "Unable to load reviews",
        );
        if (mode === "navigation") handleNavigationError(requestedView);
      } finally {
        if (mode === "manual") {
          const remaining = MIN_REFRESH_FEEDBACK_MS - (Date.now() - startedAt);
          if (remaining > 0) {
            await new Promise((resolve) => window.setTimeout(resolve, remaining));
          }
          setIsRefreshing(false);
        }
        if (requestId === requestSequence.current && !canonicalizedPage) {
          setIsLoading(false);
          finishLoad();
        }
      }
    },
    [
      canonicalizePage,
      finishLoad,
      handleNavigationError,
      handleSuccessfulLoad,
      view,
    ],
  );

  useEffect(() => {
    if (consumeSkippedViewLoad(view)) return;
    const controller = new AbortController();
    void loadDashboard(
      reviewsData ? "navigation" : "initial",
      controller.signal,
    );
    return () => controller.abort();
    // Existing cards intentionally remain visible while a new view loads.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view.hours, view.page, view.scores, loadDashboard]);

  return {
    appData,
    reviewsData,
    isLoading,
    isRefreshing,
    requestError,
    loadDashboard,
  };
}
