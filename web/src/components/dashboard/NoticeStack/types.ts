import type {
  AppResponse,
  ReviewsResponse,
} from "../../../shared/reviews/types";

export type NoticeStackProps = {
  isRefreshing: boolean;
  requestError: string | null;
  reviewsData: ReviewsResponse | null;
  sync: AppResponse["sync"] | undefined;
  hasCachedData: boolean;
};
