import { DashboardContent } from "./components/dashboard/DashboardContent/DashboardContent";
import { SparkIcon } from "./components/icons/SparkIcon";
import { useDashboard } from "./hooks/useDashboard";

export default function App() {
  const dashboard = useDashboard();
  return (
    <div className="app-shell">
      <a
        className="skip-link"
        href="#reviews-feed"
        onClick={dashboard.handleSkipLink}
      >
        Skip to reviews
      </a>
      <header className="site-header">
        <div className="container site-header__inner">
          <a className="brand" href="/" aria-label="Recent iOS reviews home">
            <span className="brand__mark">
              <SparkIcon />
            </span>
            Review pulse
          </a>
          <span className="live-chip">
            <span className="live-chip__dot" aria-hidden="true" />
            Automatic updates
          </span>
        </div>
      </header>
      <DashboardContent
        appData={dashboard.appData}
        reviewsData={dashboard.reviewsData}
        view={dashboard.view}
        loading={dashboard.loading}
        requestError={dashboard.requestError}
        feedRef={dashboard.feedRef}
        actions={dashboard.actions}
      />
      <footer className="site-footer">
        <div className="container">
          <p>Reviews are collected from Apple’s public App Store RSS feed.</p>
        </div>
      </footer>
    </div>
  );
}
