import { formatDate } from "../../../shared/reviews/formatters";
import type { ReviewCardProps } from "./types";
import { StarRating } from "../StarRating/StarRating";

export function ReviewCard({ review }: ReviewCardProps) {
  return (
    <article className="review-card">
      <div className="review-card__topline">
        <StarRating score={review.score} />
        <time dateTime={review.submittedAt}>
          {formatDate(review.submittedAt)}
        </time>
      </div>
      <h3>{review.title || "Untitled review"}</h3>
      <p className="review-card__content">{review.content}</p>
      <p className="review-card__author">By {review.author}</p>
    </article>
  );
}
