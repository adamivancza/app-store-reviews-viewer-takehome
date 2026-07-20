import { fireEvent, render, screen } from "@testing-library/react";
import { loading, reviewsResponse, view } from "../../../test/fixtures";
import { ReviewsFeed } from "./ReviewsFeed";

function renderFeed(overrides: Partial<React.ComponentProps<typeof ReviewsFeed>> = {}) {
  const props = {
    reviewsData: reviewsResponse(),
    view: view(),
    requestError: null,
    hasCachedData: true,
    showCatchingUpSkeleton: false,
    currentWindowLabel: "48 hours",
    totalItems: 2,
    totalPages: 1,
    displayedPage: 1,
    loading: loading(),
    feedRef: { current: null },
    onSelectPage: vi.fn(),
    onRetry: vi.fn(),
    ...overrides,
  };
  render(<ReviewsFeed {...props} />);
  return props;
}

describe("ReviewsFeed", () => {
  it("renders loading skeletons before data exists", () => {
    renderFeed({ reviewsData: null, hasCachedData: false, loading: loading({ initial: true }) });
    expect(screen.getByRole("status", { name: "Loading reviews" })).toBeInTheDocument();
  });

  it("keeps a skeleton during catch-up without saved reviews", () => {
    renderFeed({ reviewsData: null, hasCachedData: false, showCatchingUpSkeleton: true });
    expect(screen.getByRole("status", { name: "Loading reviews" })).toBeInTheDocument();
  });

  it("keeps saved cards visible while the service catches up", () => {
    renderFeed({ showCatchingUpSkeleton: false });
    expect(screen.getByText("The recommendations have been excellent.")).toBeInTheDocument();
  });

  it("offers a retryable initial error", () => {
    const props = renderFeed({ reviewsData: null, hasCachedData: false, requestError: "offline" });
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(props.onRetry).toHaveBeenCalledOnce();
  });

  it("guides users when no ratings are selected", () => {
    renderFeed({ reviewsData: null, view: view({ scores: [] }), hasCachedData: false });
    expect(screen.getByRole("heading", { name: "No ratings selected" })).toBeInTheDocument();
  });

  it("explains an empty filtered result", () => {
    renderFeed({ reviewsData: reviewsResponse({ reviews: [] }), view: view({ scores: [5] }) });
    expect(screen.getByRole("heading", { name: "No reviews match these ratings" })).toBeInTheDocument();
  });

  it("explains an all-ratings empty result", () => {
    renderFeed({ reviewsData: reviewsResponse({ reviews: [] }) });
    expect(screen.getByRole("heading", { name: "No reviews in this window" })).toBeInTheDocument();
    expect(screen.getByText(/Try a longer time window/)).toBeInTheDocument();
  });

  it("paginates and disables unavailable directions", () => {
    const props = renderFeed({
      totalPages: 3,
      displayedPage: 2,
      reviewsData: reviewsResponse({ pagination: { page: 2, pageSize: 25, totalItems: 70, totalPages: 3 } }),
    });
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(props.onSelectPage).toHaveBeenNthCalledWith(1, 1);
    expect(props.onSelectPage).toHaveBeenNthCalledWith(2, 3);
  });

  it("locks pagination while a page request is pending", () => {
    renderFeed({ totalPages: 3, displayedPage: 2, loading: loading({ pageChanging: true }) });
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });
});
