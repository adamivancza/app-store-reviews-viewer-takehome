import { act, renderHook } from "@testing-library/react";
import { useDashboardPolling } from "./useDashboardPolling";

const view = { hours: 48, page: 1, scores: [5, 4, 3, 2, 1] };

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("useDashboardPolling", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("polls every three seconds while catching up and every minute after", async () => {
    const loadDashboard = vi.fn().mockResolvedValue(undefined);
    const { rerender } = renderHook(
      ({ status }) => useDashboardPolling(view, status, loadDashboard),
      { initialProps: { status: "catching_up" as string | undefined } },
    );
    await act(async () => vi.advanceTimersByTimeAsync(3_000));
    expect(loadDashboard).toHaveBeenCalledTimes(1);

    rerender({ status: "current" });
    await act(async () => vi.advanceTimersByTimeAsync(59_999));
    expect(loadDashboard).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(loadDashboard).toHaveBeenCalledTimes(2);
    expect(loadDashboard).toHaveBeenLastCalledWith("automatic", expect.any(AbortSignal));
  });

  it("waits for a slow request to finish before scheduling the next poll", async () => {
    const first = deferred();
    const loadDashboard = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValue(undefined);
    renderHook(() => useDashboardPolling(view, "catching_up", loadDashboard));

    await act(async () => vi.advanceTimersByTimeAsync(3_000));
    expect(loadDashboard).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(loadDashboard).toHaveBeenCalledTimes(1);

    await act(async () => first.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(2_999));
    expect(loadDashboard).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(loadDashboard).toHaveBeenCalledTimes(2);
  });
});
