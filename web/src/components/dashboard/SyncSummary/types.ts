import type { AppResponse } from "../../../shared/reviews/types";

export type SyncSummaryProps = {
  appData: AppResponse | null;
  currentWindowLabel: string;
  totalItems: number;
};
