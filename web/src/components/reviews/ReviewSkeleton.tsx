export function ReviewSkeleton() {
  return (
    <div className="skeleton-list" role="status" aria-label="Loading reviews">
      <span className="sr-only">Loading reviews</span>
      {[0, 1, 2].map((item) => (
        <div
          className="review-card skeleton-card"
          aria-hidden="true"
          key={item}
        >
          <div className="skeleton skeleton--meta" />
          <div className="skeleton skeleton--title" />
          <div className="skeleton skeleton--line" />
          <div className="skeleton skeleton--line skeleton--line-short" />
        </div>
      ))}
    </div>
  );
}
