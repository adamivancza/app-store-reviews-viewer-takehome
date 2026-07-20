import type { AppResponse, ReviewsResponse } from "../shared/reviews/types";

type APIErrorBody = {
  error?: {
    message?: string;
  };
};

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json" },
    signal,
  });

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`;

    try {
      const body = (await response.json()) as APIErrorBody;
      if (body.error?.message) message = body.error.message;
    } catch {
      // Keep the useful HTTP fallback when the response is not JSON.
    }

    throw new Error(message);
  }

  return (await response.json()) as T;
}

export function getApp(signal?: AbortSignal): Promise<AppResponse> {
  return getJSON<AppResponse>("/api/app", signal);
}

export function getReviews(
  hours: number,
  page: number,
  scores: number[],
  signal?: AbortSignal,
): Promise<ReviewsResponse> {
  const params = new URLSearchParams({
    hours: String(hours),
    page: String(page),
    pageSize: "25",
  });
  if (scores.length !== 5) params.set("scores", scores.join(","));

  return getJSON<ReviewsResponse>(`/api/reviews?${params.toString()}`, signal);
}
