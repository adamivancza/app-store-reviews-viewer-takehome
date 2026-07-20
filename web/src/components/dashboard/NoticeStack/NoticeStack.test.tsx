import { render, screen } from "@testing-library/react";
import { appResponse, reviewsResponse } from "../../../test/fixtures";
import { NoticeStack } from "./NoticeStack";

function renderNotices(overrides: Partial<React.ComponentProps<typeof NoticeStack>> = {}) {
  render(
    <NoticeStack
      isRefreshing={false}
      requestError={null}
      reviewsData={reviewsResponse()}
      hasCachedData
      sync={appResponse().sync}
      {...overrides}
    />,
  );
}

describe("NoticeStack", () => {
  it("announces a catch-up while saved reviews remain available", () => {
    renderNotices({ sync: { ...appResponse().sync, status: "catching_up" } });
    expect(screen.getByText("Catching up after downtime")).toBeInTheDocument();
  });

  it("shows a saved-data warning for a refresh failure", () => {
    renderNotices({ requestError: "upstream request failed: secret detail" });
    expect(screen.getByRole("status")).toHaveTextContent("Showing saved reviews");
    expect(screen.getByRole("status")).toHaveTextContent("The latest refresh failed.");
    expect(screen.getByRole("status")).not.toHaveTextContent("secret detail");
  });

  it("shows the backend sync error when a refresh error is unavailable", () => {
    renderNotices({
      sync: { ...appResponse().sync, lastError: "Apple is unavailable" },
    });
    expect(screen.getByRole("status")).toHaveTextContent("The latest refresh failed.");
    expect(screen.getByRole("status")).not.toHaveTextContent("Apple is unavailable");
  });

  it("explains a historical gap", () => {
    renderNotices({
      sync: {
        ...appResponse().sync,
        historyGap: { detectedAt: "2026-07-17T08:00:00Z", after: null, before: null },
      },
    });
    expect(screen.getByText("Some historical reviews may be missing")).toBeInTheDocument();
  });

  it("includes the limiting timestamp for incomplete coverage", () => {
    renderNotices({
      reviewsData: reviewsResponse({
        coverage: { complete: false, limitedBefore: "2026-07-16T08:00:00Z" },
      }),
    });
    expect(screen.getByText("Review coverage is incomplete")).toBeInTheDocument();
    expect(screen.getByText(/Stored coverage is continuous from/)).toHaveTextContent("Jul");
    expect(screen.getByText(/Stored coverage is continuous from/).querySelector("time")).toHaveAttribute("dateTime", "2026-07-16T08:00:00Z");
  });
});
