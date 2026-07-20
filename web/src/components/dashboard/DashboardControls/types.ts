import type { AppResponse, ViewState } from "../../../shared/reviews/types";

export type DashboardControlsProps = {
  app: AppResponse["app"] | undefined;
  view: ViewState;
  isRefreshing: boolean;
  onSelectHours: (hours: number) => void;
  onToggleScore: (score: number) => void;
  onRefresh: () => void;
};
