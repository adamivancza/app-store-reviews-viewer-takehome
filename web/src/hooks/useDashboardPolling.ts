import { useEffect } from "react";
import {
  CATCH_UP_POLL_INTERVAL_MS,
  STEADY_POLL_INTERVAL_MS,
} from "./constants";
import type { ViewState } from "../shared/reviews/types";
import type { LoadMode } from "./types";

export function useDashboardPolling(
  view: ViewState,
  syncStatus: string | undefined,
  loadDashboard: (mode: LoadMode, signal?: AbortSignal) => Promise<void>,
) {
  useEffect(() => {
    const intervalMs =
      syncStatus === "catching_up"
        ? CATCH_UP_POLL_INTERVAL_MS
        : STEADY_POLL_INTERVAL_MS;
    let cancelled = false;
    let timeout: number | undefined;
    let controller: AbortController | null = null;
    const scheduleNextLoad = () => {
      timeout = window.setTimeout(async () => {
        controller = new AbortController();
        await loadDashboard("automatic", controller.signal);
        controller = null;
        if (!cancelled) scheduleNextLoad();
      }, intervalMs);
    };
    scheduleNextLoad();
    return () => {
      cancelled = true;
      if (timeout !== undefined) window.clearTimeout(timeout);
      controller?.abort();
    };
  }, [loadDashboard, syncStatus, view.hours, view.page, view.scores]);
}
