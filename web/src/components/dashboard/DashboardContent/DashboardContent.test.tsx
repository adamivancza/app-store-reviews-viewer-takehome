import { render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { appResponse, loading, reviewsResponse, view } from "../../../test/fixtures";
import { DashboardContent } from "./DashboardContent";

function renderContent(
  overrides: Partial<ComponentProps<typeof DashboardContent>> = {},
) {
  render(
    <DashboardContent
      appData={appResponse()}
      reviewsData={reviewsResponse()}
      view={view()}
      loading={loading()}
      requestError={null}
      feedRef={{ current: null }}
      actions={{
        selectHours: vi.fn(),
        selectPage: vi.fn(),
        toggleScore: vi.fn(),
        refresh: vi.fn(),
        retry: vi.fn(),
      }}
      {...overrides}
    />,
  );
}

describe("DashboardContent", () => {
  it("keeps the cached window copy until replacement data arrives", () => {
    renderContent({ view: view({ hours: 168 }) });
    expect(screen.getByText("2 results in the last 48 hours")).toBeInTheDocument();
    expect(screen.getByText("In 48 hours")).toBeInTheDocument();
  });

  it("renders a skeleton while a catch-up has no saved reviews", () => {
    renderContent({
      appData: appResponse({ sync: { ...appResponse().sync, status: "catching_up" } }),
      reviewsData: null,
    });
    expect(screen.getByRole("status", { name: "Loading reviews" })).toBeInTheDocument();
  });

  it("keeps saved cards visible while the service catches up", () => {
    renderContent({
      appData: appResponse({ sync: { ...appResponse().sync, status: "catching_up" } }),
    });
    expect(screen.getByText("The recommendations have been excellent.")).toBeInTheDocument();
  });

  it("keeps cached cards and explains a failed refresh", () => {
    renderContent({ requestError: "offline" });
    expect(screen.getByText("The recommendations have been excellent.")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Showing saved reviews");
  });
});
