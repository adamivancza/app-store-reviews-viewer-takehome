import type { StarRatingProps } from "./types";

export function StarRating({ score }: StarRatingProps) {
  const normalizedScore = Math.max(0, Math.min(5, score));
  return (
    <span
      className="rating"
      role="img"
      aria-label={`${normalizedScore} out of 5`}
    >
      {Array.from({ length: 5 }, (_, index) => (
        <svg
          className={index < normalizedScore ? "star star--filled" : "star"}
          viewBox="0 0 20 20"
          aria-hidden="true"
          key={index}
        >
          <path d="m10 1.8 2.45 4.96 5.48.8-3.97 3.86.94 5.46L10 14.3l-4.9 2.58.94-5.46-3.97-3.87 5.48-.79L10 1.8Z" />
        </svg>
      ))}
    </span>
  );
}
