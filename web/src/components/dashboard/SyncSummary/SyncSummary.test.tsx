import { render, screen } from "@testing-library/react";
import { appResponse } from "../../../test/fixtures";
import { SyncSummary } from "./SyncSummary";

describe("SyncSummary", () => {
  it("does not render before app metadata exists", () => {
    const { container } = render(<SyncSummary appData={null} currentWindowLabel="48 hours" totalItems={0} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("summarizes the visible result count and current status", () => {
    render(<SyncSummary appData={appResponse()} currentWindowLabel="48 hours" totalItems={1} />);
    expect(screen.getByText("In 48 hours")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText("review found")).toBeInTheDocument();
    expect(screen.getByText("Up to date")).toBeInTheDocument();
    expect(screen.getByText("2 stored total")).toBeInTheDocument();
  });

  it.each([
    ["catching_up", "Catching up"],
    ["gap_detected", "History gap"],
    ["error", "Needs attention"],
  ] as const)("labels %s sync status", (status, label) => {
    render(<SyncSummary appData={appResponse({ sync: { ...appResponse().sync, status } })} currentWindowLabel="7 days" totalItems={2} />);
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(screen.getByText("reviews found")).toBeInTheDocument();
  });

  it("describes a service that has not completed its first sync", () => {
    render(
      <SyncSummary
        appData={appResponse({ sync: { ...appResponse().sync, lastSuccessAt: null, lastAttemptAt: null } })}
        currentWindowLabel="48 hours"
        totalItems={0}
      />,
    );
    expect(screen.getByText("Not yet synced")).toBeInTheDocument();
    expect(screen.getByText("Waiting for first poll")).toBeInTheDocument();
  });
});
