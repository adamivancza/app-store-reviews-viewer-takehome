import { RATINGS, WINDOWS } from "../../../shared/reviews/constants";
import type { DashboardControlsProps } from "./types";
import { RefreshIcon } from "../../icons/RefreshIcon";

export function DashboardControls({
  app,
  view,
  isRefreshing,
  onSelectHours,
  onToggleScore,
  onRefresh,
}: DashboardControlsProps) {
  const allRatingsSelected = view.scores.length === RATINGS.length;
  return (
    <>
      <section className="hero" aria-labelledby="page-title">
        <div className="hero__copy">
          <p className="eyebrow">App Store intelligence</p>
          <h1 id="page-title">
            {app ? `${app.name} reviews` : "Recent iOS reviews"}
          </h1>
          <p className="hero__description">
            A focused view of what customers are saying right now, newest first.
          </p>
          {app && (
            <p className="app-identity">
              App ID {app.appId} <span aria-hidden="true">&middot;</span>{" "}
              {app.country.toUpperCase()} storefront
            </p>
          )}
        </div>
        <div className="hero__actions">
          <div
            className="window-control"
            role="group"
            aria-label="Review time window"
          >
            {WINDOWS.map((option) => (
              <button
                className="window-control__button"
                type="button"
                aria-label={option.label}
                aria-pressed={view.hours === option.hours}
                onClick={() => onSelectHours(option.hours)}
                key={option.hours}
              >
                <span className="window-control__short">
                  {option.shortLabel}
                </span>
                <span className="window-control__long">{option.label}</span>
              </button>
            ))}
          </div>
          <button
            className="refresh-button"
            type="button"
            onClick={onRefresh}
            disabled={isRefreshing}
            aria-busy={isRefreshing}
          >
            <RefreshIcon />
            Refresh
          </button>
        </div>
      </section>
      <fieldset className="rating-filter" aria-describedby="rating-filter-help">
        <legend>Filter by rating</legend>
        <div className="rating-filter__meta">
          <p id="rating-filter-help">Choose one or more scores to include.</p>
          <span>
            {allRatingsSelected
              ? "All ratings"
              : `${view.scores.length} of ${RATINGS.length} selected`}
          </span>
        </div>
        <div className="rating-filter__options">
          {RATINGS.map((score) => (
            <label className="rating-filter__option" key={score}>
              <input
                type="checkbox"
                checked={view.scores.includes(score)}
                onChange={() => onToggleScore(score)}
              />
              <span>{score === 1 ? "1 star" : `${score} stars`}</span>
            </label>
          ))}
        </div>
      </fieldset>
    </>
  );
}
