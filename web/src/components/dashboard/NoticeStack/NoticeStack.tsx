import { formatDate } from "../../../shared/reviews/formatters";
import type { NoticeStackProps } from "./types";
import { InfoIcon } from "../../icons/InfoIcon";
import { WarningIcon } from "../../icons/WarningIcon";

export function NoticeStack({
  isRefreshing,
  requestError,
  reviewsData,
  sync,
  hasCachedData,
}: NoticeStackProps) {
  const showSyncWarning =
    hasCachedData && (requestError !== null || sync?.lastError);
  return (
    <div className="notice-stack" aria-live="polite">
      <span className="sr-only">
        {isRefreshing
          ? "Refreshing reviews"
          : requestError
            ? "Review refresh failed"
            : reviewsData
              ? `Reviews updated at ${formatDate(reviewsData.generatedAt)}`
              : ""}
      </span>
      {sync?.status === "catching_up" && (
        <div className="notice notice--info">
          <InfoIcon />
          <div>
            <strong>Catching up after downtime</strong>
            <p>
              Saved reviews are available while the service checks older feed
              pages.
            </p>
          </div>
        </div>
      )}
      {showSyncWarning && (
        <div className="notice notice--warning" role="status">
          <WarningIcon />
          <div>
            <strong>Showing saved reviews</strong>
            <p>
              The latest refresh failed. Try again when the service is
              available.
            </p>
          </div>
        </div>
      )}
      {sync?.historyGap && (
        <div className="notice notice--warning" role="status">
          <WarningIcon />
          <div>
            <strong>Some historical reviews may be missing</strong>
            <p>
              The available App Store pages did not reach the last saved
              checkpoint. New reviews will continue to sync normally.
            </p>
          </div>
        </div>
      )}
      {reviewsData && !reviewsData.coverage.complete && (
        <div className="notice notice--warning" role="status">
          <WarningIcon />
          <div>
            <strong>Review coverage is incomplete</strong>
            <p>
              {reviewsData.coverage.limitedBefore ? (
                <>
                  Stored coverage is continuous from{" "}
                  <time dateTime={reviewsData.coverage.limitedBefore}>
                    {formatDate(reviewsData.coverage.limitedBefore)}
                  </time>
                  . Earlier reviews in this window may be missing.
                </>
              ) : (
                "The App Store feed does not provide complete coverage for this window. Some older reviews may be missing."
              )}
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
