#!/usr/bin/env python3
"""Browser assertions for the compiled React app and real Go API."""

from __future__ import annotations

import sys
from datetime import datetime
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import sync_playwright


def query(page_url: str) -> dict[str, list[str]]:
    return parse_qs(urlparse(page_url).query)


def main(base_url: str, submitted_at: str, older_submitted_at: str) -> None:
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1280, "height": 900})
        console_errors: list[str] = []
        page.on("console", lambda message: console_errors.append(message.text) if message.type == "error" else None)
        page.on("pageerror", lambda error: console_errors.append(str(error)))

        page.goto(base_url)
        page.wait_for_load_state("networkidle")

        try:
            page.get_by_role("heading", name="A deterministic delight").wait_for()
        except Exception:
            print(page.locator("body").inner_text(), file=sys.stderr)
            print(f"Browser errors: {console_errors}", file=sys.stderr)
            raise
        featured_card = page.locator(".review-card").filter(
            has=page.get_by_role("heading", name="A deterministic delight")
        )
        assert page.get_by_text(
            "The complete E2E review body survives storage, API, and React rendering."
        ).is_visible()
        assert page.get_by_text("By E2E Reviewer").is_visible()
        assert featured_card.get_by_label("5 out of 5").is_visible()
        submitted_time = page.locator(f'time[datetime="{submitted_at}"]')
        assert submitted_time.is_visible()
        assert submitted_time.inner_text().strip()

        cards = page.locator(".review-card")
        assert cards.count() == 25
        assert cards.first.get_by_role("heading").inner_text() == "A deterministic delight"
        assert cards.last.get_by_role("heading").inner_text() == "Recent review 25"
        submitted_times = [
            datetime.fromisoformat(value.replace("Z", "+00:00"))
            for value in cards.locator("time").evaluate_all(
                "elements => elements.map(element => element.getAttribute('datetime'))"
            )
        ]
        assert submitted_times == sorted(submitted_times, reverse=True)

        api_response = page.request.get(f"{base_url}/api/reviews?hours=48&page=1&pageSize=25&scores=5")
        assert api_response.ok
        api_body = api_response.json()
        assert api_body["pagination"]["totalItems"] == 6
        assert api_body["reviews"][0]["id"] == "fixture-five-star"
        assert all(review["score"] == 5 for review in api_body["reviews"])

        live_response = page.request.get(f"{base_url}/api/live")
        assert live_response.ok
        assert live_response.json()["status"] == "ok"
        ready_response = page.request.get(f"{base_url}/api/ready")
        assert ready_response.ok
        assert ready_response.json()["status"] == "ready"
        freshness_response = page.request.get(f"{base_url}/api/freshness")
        assert freshness_response.ok
        freshness = freshness_response.json()
        assert freshness["status"] in {"current", "updating", "stale"}
        assert freshness["complete"] is True

        next_button = page.get_by_role("button", name="Next")
        previous_button = page.get_by_role("button", name="Previous")
        next_button.click()
        page.wait_for_url("**?hours=48&page=2")
        page.get_by_role("heading", name="Recent review 26").wait_for()
        assert page.locator(".review-card").count() == 1
        assert query(page.url)["page"] == ["2"]

        previous_button.click()
        page.wait_for_url("**?hours=48&page=1")
        page.get_by_role("heading", name="A deterministic delight").wait_for()
        assert page.locator(".review-card").count() == 25

        page.go_back()
        page.wait_for_url("**?hours=48&page=2")
        page.get_by_role("heading", name="Recent review 26").wait_for()
        assert page.locator(".review-card").count() == 1
        assert query(page.url)["page"] == ["2"]

        page.go_forward()
        page.wait_for_url("**?hours=48&page=1")
        page.get_by_role("heading", name="A deterministic delight").wait_for()
        assert page.locator(".review-card").count() == 25
        assert query(page.url)["page"] == ["1"]

        one_star = page.get_by_role("checkbox", name="1 star")
        assert one_star.is_checked()
        one_star.uncheck()
        page.get_by_text("20 results in the last 48 hours").wait_for()
        assert page.get_by_role("heading", name="A deterministic delight").is_visible()
        assert page.get_by_role("heading", name="Filtered fixture review").count() == 0
        assert page.get_by_label("1 out of 5").count() == 0

        filtered_query = query(page.url)
        assert filtered_query["page"] == ["1"]
        assert filtered_query["scores"] == ["5,4,3,2"]

        page.get_by_role("button", name="7 days").click()
        page.wait_for_url("**?hours=168&page=1&scores=5%2C4%2C3%2C2")
        page.get_by_text("21 results in the last 7 days").wait_for()
        page.get_by_role("heading", name="Worth finding after 48 hours").wait_for()
        assert page.get_by_text(
            "This review appears only after switching to the seven-day window."
        ).is_visible()
        assert page.locator(f'time[datetime="{older_submitted_at}"]').is_visible()

        assert not console_errors, console_errors
        browser.close()

        print(
            "E2E passed: persisted snapshot -> ops/API -> newest-first pagination/history -> filters -> 7-day window"
        )


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3])
