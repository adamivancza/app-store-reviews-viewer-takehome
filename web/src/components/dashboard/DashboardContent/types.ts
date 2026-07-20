import type { RefObject } from "react";
import type {
  AppResponse,
  DashboardActions,
  DashboardLoadingState,
  ReviewsResponse,
  ViewState,
} from "../../../shared/reviews/types";

export type DashboardContentProps = {
  appData: AppResponse | null;
  reviewsData: ReviewsResponse | null;
  view: ViewState;
  loading: DashboardLoadingState;
  requestError: string | null;
  feedRef: RefObject<HTMLElement | null>;
  actions: DashboardActions;
};
