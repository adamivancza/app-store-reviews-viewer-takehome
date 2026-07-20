import { formatDate } from "../../../shared/reviews/formatters";
import type { SyncSummaryProps } from "./types";

export function SyncSummary({
  appData,
  currentWindowLabel,
  totalItems,
}: SyncSummaryProps) {
  if (!appData) return null;
  const sync = appData.sync;
  return (
    <section className="stats" aria-label="Review sync summary">
      <div className="stat">
        <span className="stat__label">In {currentWindowLabel}</span>
        <strong className="stat__value">{totalItems}</strong>
        <span className="stat__detail">
          {totalItems === 1 ? "review" : "reviews"} found
        </span>
      </div>
      <div className="stat">
        <span className="stat__label">Sync status</span>
        <strong className={`status-value status-value--${sync.status}`}>
          <span className="status-value__dot" aria-hidden="true" />
          {sync.status === "catching_up"
            ? "Catching up"
            : sync.status === "gap_detected"
              ? "History gap"
              : sync.status === "error"
                ? "Needs attention"
                : "Up to date"}
        </strong>
        <span className="stat__detail">{appData.reviewCount} stored total</span>
      </div>
      <div className="stat">
        <span className="stat__label">Last successful sync</span>
        {sync.lastSuccessAt ? (
          <strong className="stat__date">
            <time dateTime={sync.lastSuccessAt}>
              {formatDate(sync.lastSuccessAt)}
            </time>
          </strong>
        ) : (
          <strong className="stat__date">Not yet synced</strong>
        )}
        <span className="stat__detail">
          {sync.lastAttemptAt ? (
            <>
              Last attempt{" "}
              <time dateTime={sync.lastAttemptAt}>
                {formatDate(sync.lastAttemptAt)}
              </time>
            </>
          ) : (
            "Waiting for first poll"
          )}
        </span>
      </div>
    </section>
  );
}
