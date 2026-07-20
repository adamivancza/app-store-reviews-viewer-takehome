import { useCallback, useEffect, useRef, useState } from "react";
import { RATINGS } from "../shared/reviews/constants";
import type { ViewState } from "../shared/reviews/types";
import { DEFAULT_PAGE } from "./constants";
import { readViewFromURL, viewKey, writeViewToURL } from "./helpers";
import type { DashboardNavigation } from "./types";

export function useDashboardNavigation(): DashboardNavigation {
  const [view, setView] = useState(readViewFromURL);
  const [isPageChanging, setIsPageChanging] = useState(false);
  const [isFilterChanging, setIsFilterChanging] = useState(false);
  const appliedView = useRef<ViewState | null>(null);
  const skipNextViewLoad = useRef<string | null>(null);
  const feedRef = useRef<HTMLElement>(null);
  const pendingFeedFocus = useRef<string | null>(null);

  const focusFeed = useCallback(() => {
    const feed = feedRef.current;
    if (!feed) return;
    feed.focus({ preventScroll: true });
    feed.scrollIntoView?.({ behavior: "smooth", block: "start" });
  }, []);

  useEffect(() => {
    const handlePopState = () => {
      const nextView = readViewFromURL();
      const isPageOnly =
        nextView.hours === view.hours &&
        nextView.scores.join(",") === view.scores.join(",");
      pendingFeedFocus.current = null;
      setIsPageChanging(true);
      setIsFilterChanging(!isPageOnly);
      setView(nextView);
    };
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, [view]);

  const selectHours = useCallback(
    (nextHours: number) => {
      if (nextHours === view.hours && view.page === DEFAULT_PAGE) return;
      pendingFeedFocus.current = null;
      const nextView = {
        hours: nextHours,
        page: DEFAULT_PAGE,
        scores: view.scores,
      };
      writeViewToURL(nextView);
      setView(nextView);
    },
    [view],
  );

  const selectPage = useCallback(
    (nextPage: number) => {
      if (nextPage === view.page || nextPage < 1) return;
      const nextView = { ...view, page: nextPage };
      pendingFeedFocus.current = viewKey(nextView);
      setIsPageChanging(true);
      writeViewToURL(nextView);
      setView(nextView);
    },
    [view],
  );

  const toggleScore = useCallback(
    (score: number) => {
      const selected = new Set(view.scores);
      if (selected.has(score)) selected.delete(score);
      else selected.add(score);
      const nextView = {
        ...view,
        page: DEFAULT_PAGE,
        scores: RATINGS.filter((rating) => selected.has(rating)),
      };
      pendingFeedFocus.current = null;
      setIsFilterChanging(true);
      writeViewToURL(nextView);
      setView(nextView);
    },
    [view],
  );

  const consumeSkippedViewLoad = useCallback((requestedView: ViewState) => {
    if (skipNextViewLoad.current !== viewKey(requestedView)) return false;
    skipNextViewLoad.current = null;
    return true;
  }, []);

  const canonicalizePage = useCallback(
    (requestedView: ViewState, page: number) => {
      const normalizedView = { ...requestedView, page };
      if (pendingFeedFocus.current === viewKey(requestedView))
        pendingFeedFocus.current = viewKey(normalizedView);
      setIsPageChanging(true);
      writeViewToURL(normalizedView, "replace");
      setView(normalizedView);
    },
    [],
  );

  const handleSuccessfulLoad = useCallback(
    (loadedView: ViewState) => {
      appliedView.current = loadedView;
      if (pendingFeedFocus.current !== viewKey(loadedView)) return;
      pendingFeedFocus.current = null;
      focusFeed();
    },
    [focusFeed],
  );

  const finishLoad = useCallback(() => {
    setIsPageChanging(false);
    setIsFilterChanging(false);
  }, []);

  const handleNavigationError = useCallback((requestedView: ViewState) => {
    pendingFeedFocus.current = null;
    const previousView = appliedView.current;
    if (!previousView || viewKey(previousView) === viewKey(requestedView))
      return;
    skipNextViewLoad.current = viewKey(previousView);
    writeViewToURL(previousView, "replace");
    setView(previousView);
  }, []);

  const handleSkipLink = useCallback(
    (event: React.MouseEvent<HTMLAnchorElement>) => {
      event.preventDefault();
      focusFeed();
    },
    [focusFeed],
  );

  return {
    view,
    feedRef,
    isPageChanging,
    isFilterChanging,
    selectHours,
    selectPage,
    toggleScore,
    handleSkipLink,
    consumeSkippedViewLoad,
    canonicalizePage,
    handleSuccessfulLoad,
    handleNavigationError,
    finishLoad,
  };
}
