import { act, renderHook } from "@testing-library/react";
import { useDashboardNavigation } from "./useDashboardNavigation";

function renderNavigation(url = "/?hours=48&page=1") {
  window.history.replaceState({}, "", url);
  return renderHook(() => useDashboardNavigation());
}

describe("useDashboardNavigation", () => {
  it("writes page navigation to the URL and focuses the feed after a successful load", () => {
    const { result } = renderNavigation();
    const feed = document.createElement("section");
    feed.tabIndex = -1;
    feed.scrollIntoView = vi.fn();
    document.body.append(feed);
    result.current.feedRef.current = feed;

    act(() => result.current.handleSuccessfulLoad(result.current.view));
    act(() => result.current.selectPage(2));
    expect(window.location.search).toBe("?hours=48&page=2");
    expect(result.current.isPageChanging).toBe(true);

    act(() => result.current.handleSuccessfulLoad(result.current.view));
    expect(feed).toHaveFocus();
    expect(feed.scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" });
    feed.remove();
  });

  it("resets to page one when the time window changes", () => {
    const { result } = renderNavigation("/?hours=48&page=3");
    act(() => result.current.selectHours(168));
    expect(result.current.view).toEqual({ hours: 168, page: 1, scores: [5, 4, 3, 2, 1] });
    expect(window.location.search).toBe("?hours=168&page=1");
  });

  it("updates the URL and resets the page when a rating is toggled", () => {
    const { result } = renderNavigation("/?hours=48&page=2");
    act(() => result.current.toggleScore(3));
    expect(result.current.view).toEqual({ hours: 48, page: 1, scores: [5, 4, 2, 1] });
    expect(window.location.search).toBe("?hours=48&page=1&scores=5%2C4%2C2%2C1");
    expect(result.current.isFilterChanging).toBe(true);
  });

  it("treats browser history between pages as page navigation", () => {
    const { result } = renderNavigation("/?hours=48&page=2");
    act(() => {
      window.history.replaceState({}, "", "/?hours=48&page=1");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(result.current.view).toEqual({ hours: 48, page: 1, scores: [5, 4, 3, 2, 1] });
    expect(result.current.isPageChanging).toBe(true);
    expect(result.current.isFilterChanging).toBe(false);
  });

  it("treats browser history with a changed filter as filter navigation", () => {
    const { result } = renderNavigation("/?hours=48&page=1");
    act(() => {
      window.history.replaceState({}, "", "/?hours=168&page=3&scores=5,4");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(result.current.view).toEqual({ hours: 168, page: 3, scores: [5, 4] });
    expect(result.current.isPageChanging).toBe(true);
    expect(result.current.isFilterChanging).toBe(true);
  });

  it("rolls a failed page navigation back to the applied view and URL", () => {
    const { result } = renderNavigation();
    act(() => result.current.handleSuccessfulLoad(result.current.view));
    act(() => result.current.selectPage(2));
    act(() => result.current.handleNavigationError(result.current.view));
    expect(result.current.view.page).toBe(1);
    expect(window.location.search).toBe("?hours=48&page=1");
    expect(result.current.consumeSkippedViewLoad(result.current.view)).toBe(true);
  });

  it("rolls a failed rating change back to the applied view and URL", () => {
    const { result } = renderNavigation("/?hours=48&page=2");
    act(() => result.current.handleSuccessfulLoad(result.current.view));
    act(() => result.current.toggleScore(1));
    act(() => result.current.handleNavigationError(result.current.view));
    expect(result.current.view).toEqual({ hours: 48, page: 2, scores: [5, 4, 3, 2, 1] });
    expect(window.location.search).toBe("?hours=48&page=2");
  });

  it("rolls a failed window change back to the applied view and URL", () => {
    const { result } = renderNavigation();
    act(() => result.current.handleSuccessfulLoad(result.current.view));
    act(() => result.current.selectHours(168));
    act(() => result.current.handleNavigationError(result.current.view));
    expect(result.current.view).toEqual({ hours: 48, page: 1, scores: [5, 4, 3, 2, 1] });
    expect(window.location.search).toBe("?hours=48&page=1");
  });

  it("canonicalizes an out-of-range page with replace-state semantics", () => {
    const { result } = renderNavigation("/?hours=48&page=9");
    act(() => result.current.canonicalizePage(result.current.view, 3));
    expect(result.current.view.page).toBe(3);
    expect(window.location.search).toBe("?hours=48&page=3");
    expect(result.current.isPageChanging).toBe(true);
  });
});
