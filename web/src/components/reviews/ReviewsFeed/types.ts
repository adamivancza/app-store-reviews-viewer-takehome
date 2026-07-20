import type { RefObject } from "react";
import type {
  DashboardLoadingState,
  ReviewsResponse,
  ViewState,
} from "../../../shared/reviews/types";

export type ReviewsFeedProps = {
  reviewsData: ReviewsResponse | null;
  view: ViewState;
  requestError: string | null;
  hasCachedData: boolean;
  showCatchingUpSkeleton: boolean;
  currentWindowLabel: string;
  totalItems: number;
  totalPages: number;
  displayedPage: number;
  loading: DashboardLoadingState;
  feedRef: RefObject<HTMLElement | null>;
  onSelectPage: (page: number) => void;
  onRetry: () => void;
};
