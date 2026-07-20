import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "./App";
import { appResponse, jsonResponse, reviewsResponse } from "./test/fixtures";

function mockDashboardFetch() {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) =>
    jsonResponse(
      String(input) === "/api/app" ? appResponse() : reviewsResponse(),
    ),
  );
}

describe("App", () => {
  it("loads the dashboard and presents the required review details", async () => {
    mockDashboardFetch();
    render(<App />);

    expect(screen.getByText("Loading reviews")).toBeInTheDocument();
    expect(await screen.findByText("The recommendations have been excellent.")).toBeInTheDocument();
    expect(screen.getByText("By Ada")).toBeInTheDocument();
    expect(screen.getByLabelText("5 out of 5")).toBeInTheDocument();
    expect(document.querySelector('time[datetime="2026-07-17T07:00:00Z"]')).toBeInTheDocument();
    expect(screen.getByText("2 results in the last 48 hours")).toBeInTheDocument();
    expect(screen.getByText("Spotify reviews")).toBeInTheDocument();
  });

  it("offers a retry after an initial error and recovers", async () => {
    let attempts = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      attempts += 1;
      if (attempts <= 2) return jsonResponse({ error: { message: "offline" } }, 503);
      return jsonResponse(
        String(input) === "/api/app" ? appResponse() : reviewsResponse(),
      );
    });
    render(<App />);

    expect(await screen.findByRole("alert")).toHaveTextContent("offline");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("The recommendations have been excellent.")).toBeInTheDocument();
  });

  it("moves keyboard focus to the feed from the skip link", async () => {
    mockDashboardFetch();
    render(<App />);

    const feed = await screen.findByRole("region", { name: "Latest reviews" });
    fireEvent.click(screen.getByRole("link", { name: "Skip to reviews" }));
    await waitFor(() => expect(feed).toHaveFocus());
  });
});
