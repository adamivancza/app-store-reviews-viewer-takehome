import { useCallback } from "react";
import type { DashboardController } from "./types";
import { useDashboardData } from "./useDashboardData";
import { useDashboardNavigation } from "./useDashboardNavigation";
import { useDashboardPolling } from "./useDashboardPolling";

export function useDashboard(): DashboardController {
  const navigation = useDashboardNavigation();
  const data = useDashboardData(navigation);
  useDashboardPolling(
    navigation.view,
    data.appData?.sync.status,
    data.loadDashboard,
  );
  const refresh = useCallback(
    () => void data.loadDashboard("manual"),
    [data.loadDashboard],
  );
  const retry = useCallback(
    () => void data.loadDashboard("initial"),
    [data.loadDashboard],
  );
  return {
    appData: data.appData,
    reviewsData: data.reviewsData,
    view: navigation.view,
    loading: {
      initial: data.isLoading,
      refreshing: data.isRefreshing,
      pageChanging: navigation.isPageChanging,
      filterChanging: navigation.isFilterChanging,
    },
    requestError: data.requestError,
    feedRef: navigation.feedRef,
    actions: {
      selectHours: navigation.selectHours,
      selectPage: navigation.selectPage,
      toggleScore: navigation.toggleScore,
      refresh,
      retry,
    },
    handleSkipLink: navigation.handleSkipLink,
  };
}
