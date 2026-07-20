import type { RefObject } from "react";
import type {
  AppResponse,
  DashboardActions,
  DashboardLoadingState,
  ReviewsResponse,
  ViewState,
} from "../shared/reviews/types";

export type LoadMode = "initial" | "automatic" | "manual" | "navigation";

export type DashboardNavigation = {
  view: ViewState;
  feedRef: RefObject<HTMLElement | null>;
  isPageChanging: boolean;
  isFilterChanging: boolean;
  selectHours: (hours: number) => void;
  selectPage: (page: number) => void;
  toggleScore: (score: number) => void;
  handleSkipLink: (event: React.MouseEvent<HTMLAnchorElement>) => void;
  consumeSkippedViewLoad: (view: ViewState) => boolean;
  canonicalizePage: (requestedView: ViewState, page: number) => void;
  handleSuccessfulLoad: (view: ViewState) => void;
  handleNavigationError: (requestedView: ViewState) => void;
  finishLoad: () => void;
};

export type DashboardData = {
  appData: AppResponse | null;
  reviewsData: ReviewsResponse | null;
  isLoading: boolean;
  isRefreshing: boolean;
  requestError: string | null;
  loadDashboard: (mode: LoadMode, signal?: AbortSignal) => Promise<void>;
};

export type DashboardController = {
  appData: AppResponse | null;
  reviewsData: ReviewsResponse | null;
  view: ViewState;
  loading: DashboardLoadingState;
  requestError: string | null;
  feedRef: RefObject<HTMLElement | null>;
  actions: DashboardActions;
  handleSkipLink: (event: React.MouseEvent<HTMLAnchorElement>) => void;
};
