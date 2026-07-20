import { RATINGS, WINDOWS } from "../shared/reviews/constants";
import type { ViewState } from "../shared/reviews/types";
import { DEFAULT_HOURS, DEFAULT_PAGE } from "./constants";

export function readScoresFromURL(params: URLSearchParams): number[] {
  if (!params.has("scores")) return [...RATINGS];
  const requested = new Set(
    (params.get("scores") ?? "")
      .split(",")
      .map(Number)
      .filter((score) => Number.isInteger(score) && score >= 1 && score <= 5),
  );
  return RATINGS.filter((score) => requested.has(score));
}

export function readViewFromURL(): ViewState {
  const params = new URLSearchParams(window.location.search);
  const hoursValue = Number(params.get("hours"));
  const pageValue = Number(params.get("page"));
  return {
    hours: WINDOWS.some((option) => option.hours === hoursValue)
      ? hoursValue
      : DEFAULT_HOURS,
    page:
      Number.isInteger(pageValue) && pageValue > 0 ? pageValue : DEFAULT_PAGE,
    scores: readScoresFromURL(params),
  };
}

export function writeViewToURL(
  view: ViewState,
  mode: "push" | "replace" = "push",
): void {
  const url = new URL(window.location.href);
  url.searchParams.set("hours", String(view.hours));
  url.searchParams.set("page", String(view.page));
  if (view.scores.length === RATINGS.length) url.searchParams.delete("scores");
  else url.searchParams.set("scores", view.scores.join(","));
  if (mode === "replace") window.history.replaceState(view, "", url);
  else window.history.pushState(view, "", url);
}

export function viewKey(view: ViewState): string {
  return `${view.hours}:${view.page}:${view.scores.join(",")}`;
}
