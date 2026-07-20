import { fireEvent, render, screen } from "@testing-library/react";
import { appResponse, view } from "../../../test/fixtures";
import { DashboardControls } from "./DashboardControls";

function renderControls(overrides: Partial<React.ComponentProps<typeof DashboardControls>> = {}) {
  const props = {
    app: appResponse().app,
    view: view(),
    isRefreshing: false,
    onSelectHours: vi.fn(),
    onToggleScore: vi.fn(),
    onRefresh: vi.fn(),
    ...overrides,
  };
  render(<DashboardControls {...props} />);
  return props;
}

describe("DashboardControls", () => {
  it("identifies the app, selected window, and all ratings", () => {
    renderControls();
    expect(screen.getByRole("heading", { name: "Spotify reviews" })).toBeInTheDocument();
    expect(document.querySelector(".app-identity")).toHaveTextContent("App ID 324684580");
    expect(screen.getByRole("button", { name: "48 hours" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("All ratings")).toBeInTheDocument();
    expect(screen.getAllByRole("checkbox", { checked: true })).toHaveLength(5);
  });

  it("delegates window and score selections", () => {
    const props = renderControls();
    fireEvent.click(screen.getByRole("button", { name: "7 days" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "1 star" }));
    expect(props.onSelectHours).toHaveBeenCalledWith(168);
    expect(props.onToggleScore).toHaveBeenCalledWith(1);
  });

  it("reports a partial rating selection", () => {
    renderControls({ view: view({ scores: [5, 3] }) });
    expect(screen.getByText("2 of 5 selected")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "4 stars" })).not.toBeChecked();
  });

  it("disables refresh while a manual request is active", () => {
    const props = renderControls({ isRefreshing: true });
    const refresh = screen.getByRole("button", { name: "Refresh" });
    expect(refresh).toBeDisabled();
    expect(refresh).toHaveAttribute("aria-busy", "true");
    fireEvent.click(refresh);
    expect(props.onRefresh).not.toHaveBeenCalled();
  });
});
