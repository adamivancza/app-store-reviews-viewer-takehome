import { RATINGS } from "../../../shared/reviews/constants";
import type { ReviewsFeedProps } from "./types";
import { EmptyIcon } from "../../icons/EmptyIcon";
import { ReviewCard } from "../ReviewCard/ReviewCard";
import { ReviewSkeleton } from "../ReviewSkeleton";
import { WarningIcon } from "../../icons/WarningIcon";

export function ReviewsFeed({
  reviewsData,
  view,
  requestError,
  hasCachedData,
  showCatchingUpSkeleton,
  currentWindowLabel,
  totalItems,
  totalPages,
  displayedPage,
  loading,
  feedRef,
  onSelectPage,
  onRetry,
}: ReviewsFeedProps) {
  const reviews = reviewsData?.reviews ?? [];
  const allRatingsSelected = view.scores.length === RATINGS.length;
  return (
    <section
      className="feed"
      id="reviews-feed"
      aria-labelledby="reviews-heading"
      aria-busy={loading.pageChanging || loading.filterChanging}
      ref={feedRef}
      tabIndex={-1}
    >
      <div className="feed__heading">
        <div>
          <p className="eyebrow">Customer voice</p>
          <h2 id="reviews-heading">Latest reviews</h2>
        </div>
        {reviewsData && (
          <p>
            {totalItems} {totalItems === 1 ? "result" : "results"} in the last{" "}
            {currentWindowLabel}
          </p>
        )}
      </div>
      {(loading.initial && !hasCachedData) ||
      showCatchingUpSkeleton ||
      loading.filterChanging ? (
        <ReviewSkeleton />
      ) : requestError && !hasCachedData ? (
        <div className="state-card state-card--error" role="alert">
          <WarningIcon />
          <h3>Reviews couldn’t be loaded</h3>
          <p>{requestError}</p>
          <button type="button" onClick={onRetry}>
            Try again
          </button>
        </div>
      ) : reviews.length === 0 ? (
        <div className="state-card">
          <EmptyIcon />
          {view.scores.length === 0 ? (
            <>
              <h3>No ratings selected</h3>
              <p>Choose at least one rating above to see matching reviews.</p>
            </>
          ) : !allRatingsSelected ? (
            <>
              <h3>No reviews match these ratings</h3>
              <p>
                Try selecting more ratings or choosing a longer time window.
              </p>
            </>
          ) : (
            <>
              <h3>No reviews in this window</h3>
              <p>
                There aren’t any stored reviews from the last{" "}
                {currentWindowLabel}. Try a longer time window or refresh after
                the next poll.
              </p>
            </>
          )}
        </div>
      ) : (
        <div className="review-list">
          {reviews.map((review) => (
            <ReviewCard review={review} key={review.id} />
          ))}
        </div>
      )}
      {reviewsData && totalPages > 0 && !loading.filterChanging && (
        <nav className="pagination" aria-label="Review pages">
          <button
            type="button"
            onClick={() => onSelectPage(displayedPage - 1)}
            disabled={loading.pageChanging || displayedPage <= 1}
          >
            Previous
          </button>
          <p className="pagination__status">
            Page <strong>{displayedPage}</strong> of{" "}
            <strong>{totalPages}</strong>
          </p>
          <button
            type="button"
            onClick={() => onSelectPage(displayedPage + 1)}
            disabled={loading.pageChanging || displayedPage >= totalPages}
          >
            Next
          </button>
        </nav>
      )}
    </section>
  );
}
