import { windowLabel } from "./helpers";
import type { DashboardContentProps } from "./types";
import { DashboardControls } from "../DashboardControls/DashboardControls";
import { NoticeStack } from "../NoticeStack/NoticeStack";
import { ReviewsFeed } from "../../reviews/ReviewsFeed/ReviewsFeed";
import { SyncSummary } from "../SyncSummary/SyncSummary";

export function DashboardContent({
  appData,
  reviewsData,
  view,
  loading,
  requestError,
  feedRef,
  actions,
}: DashboardContentProps) {
  const reviews = reviewsData?.reviews ?? [];
  const hasCachedData = appData !== null && reviewsData !== null;
  const app = appData?.app ?? reviewsData?.app;
  const sync = appData?.sync;
  const displayedHours = reviewsData?.window.hours ?? view.hours;
  const currentWindowLabel = windowLabel(displayedHours);
  const totalItems = reviewsData?.pagination.totalItems ?? 0;
  const totalPages = reviewsData?.pagination.totalPages ?? 0;
  const displayedPage = reviewsData?.pagination.page ?? view.page;
  const showCatchingUpSkeleton =
    sync?.status === "catching_up" && reviews.length === 0;
  return (
    <main className="container">
      <DashboardControls
        app={app}
        view={view}
        isRefreshing={loading.refreshing}
        onSelectHours={actions.selectHours}
        onToggleScore={actions.toggleScore}
        onRefresh={actions.refresh}
      />
      <SyncSummary
        appData={appData}
        currentWindowLabel={currentWindowLabel}
        totalItems={totalItems}
      />
      <NoticeStack
        isRefreshing={loading.refreshing}
        requestError={requestError}
        reviewsData={reviewsData}
        sync={sync}
        hasCachedData={hasCachedData}
      />
      <ReviewsFeed
        reviewsData={reviewsData}
        view={view}
        requestError={requestError}
        hasCachedData={hasCachedData}
        showCatchingUpSkeleton={showCatchingUpSkeleton}
        currentWindowLabel={currentWindowLabel}
        totalItems={totalItems}
        totalPages={totalPages}
        displayedPage={displayedPage}
        loading={loading}
        feedRef={feedRef}
        onSelectPage={actions.selectPage}
        onRetry={actions.retry}
      />
    </main>
  );
}
