export type App = {
  key: string;
  name: string;
  appId: string;
  country: string;
};

export type HistoryGap = {
  detectedAt: string;
  after: string | null;
  before: string | null;
};

export type SyncStatus = "current" | "catching_up" | "gap_detected" | "error";

export type AppResponse = {
  app: App;
  reviewCount: number;
  sync: {
    status: SyncStatus;
    lastAttemptAt: string | null;
    lastSuccessAt: string | null;
    lastError: string | null;
    historyGap: HistoryGap | null;
  };
};

export type Review = {
  id: string;
  title: string;
  content: string;
  author: string;
  score: number;
  submittedAt: string;
};

export type ReviewsResponse = {
  app: App;
  generatedAt: string;
  window: {
    hours: number;
    from: string;
    to: string;
  };
  pagination: {
    page: number;
    pageSize: number;
    totalItems: number;
    totalPages: number;
  };
  coverage: {
    complete: boolean;
    limitedBefore: string | null;
  };
  reviews: Review[];
};

export type ViewState = { hours: number; page: number; scores: number[] };

export type DashboardLoadingState = {
  initial: boolean;
  refreshing: boolean;
  pageChanging: boolean;
  filterChanging: boolean;
};

export type DashboardActions = {
  selectHours: (hours: number) => void;
  selectPage: (page: number) => void;
  toggleScore: (score: number) => void;
  refresh: () => void;
  retry: () => void;
};
