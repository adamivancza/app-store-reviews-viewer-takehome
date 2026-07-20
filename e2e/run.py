#!/usr/bin/env python3
"""Run the production browser test against an isolated persisted snapshot."""

from __future__ import annotations

import json
import os
from pathlib import Path
import shlex
import socket
import subprocess
import sys
import tempfile
from datetime import datetime, timedelta, timezone


ROOT = Path(__file__).resolve().parents[1]
SERVER = ROOT / "bin" / "reviews-viewer"
FRONTEND = ROOT / "web" / "dist" / "index.html"
WITH_SERVER = ROOT / "e2e" / "with_server.py"


def available_port() -> int:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def iso_z(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def main() -> int:
    if not SERVER.is_file() or not FRONTEND.is_file():
        raise SystemExit("E2E prerequisites are missing; run `make build` first")
    if not WITH_SERVER.is_file():
        raise SystemExit(f"E2E server helper not found at {WITH_SERVER}")

    port = available_port()
    now = datetime.now(timezone.utc).replace(microsecond=0)
    submitted_at = iso_z(now - timedelta(hours=1))
    older_submitted_at = iso_z(now - timedelta(hours=72))
    fetched_at = iso_z(now - timedelta(minutes=30))

    recent_reviews = [
        {
            "id": "fixture-five-star",
            "appKey": "e2e-app",
            "title": "A deterministic delight",
            "content": "The complete E2E review body survives storage, API, and React rendering.",
            "author": "E2E Reviewer",
            "score": 5,
            "submittedAt": submitted_at,
            "fetchedAt": fetched_at,
        },
        {
            "id": "fixture-one-star",
            "appKey": "e2e-app",
            "title": "Filtered fixture review",
            "content": "This lower-rated review proves the real API applies score filters.",
            "author": "Filter Reviewer",
            "score": 1,
            "submittedAt": iso_z(now - timedelta(hours=2)),
            "fetchedAt": fetched_at,
        },
    ]
    for position in range(3, 27):
        recent_reviews.append(
            {
                "id": f"fixture-recent-{position:02d}",
                "appKey": "e2e-app",
                "title": f"Recent review {position:02d}",
                "content": f"Deterministic recent review body {position:02d}.",
                "author": f"Recent Reviewer {position:02d}",
                "score": ((position - 1) % 5) + 1,
                "submittedAt": iso_z(now - timedelta(hours=position)),
                "fetchedAt": fetched_at,
            }
        )

    older_review = {
        "id": "fixture-older-than-48h",
        "appKey": "e2e-app",
        "title": "Worth finding after 48 hours",
        "content": "This review appears only after switching to the seven-day window.",
        "author": "Earlier Reviewer",
        "score": 2,
        "submittedAt": older_submitted_at,
        "fetchedAt": fetched_at,
    }

    with tempfile.TemporaryDirectory(prefix="reviews-viewer-e2e-") as temp:
        temp_dir = Path(temp)
        data_dir = temp_dir / "data"
        data_dir.mkdir()
        config_path = temp_dir / "config.json"
        snapshot_path = data_dir / "e2e-app.json"

        config = {
            "key": "e2e-app",
            "name": "E2E Reviews",
            "appId": "123456789",
            "country": "us",
            "pollInterval": "24h",
            "maxPages": 1,
            "dataDir": str(data_dir),
            "listenAddr": f"127.0.0.1:{port}",
        }
        snapshot = {
            "version": 1,
            "app": {
                "key": "e2e-app",
                "name": "E2E Reviews",
                "appId": "123456789",
                "country": "us",
            },
            # Seed oldest-first so the real JSON store load must establish the
            # newest-first invariant before the API and browser consume it.
            "reviews": list(reversed([*recent_reviews, older_review])),
            "sync": {
                "status": "current",
                "lastAttemptAt": fetched_at,
                "lastSuccessAt": fetched_at,
                "lastError": None,
                "historyGap": None,
                "catchUp": None,
                "historyLimit": None,
            },
        }
        config_path.write_text(json.dumps(config), encoding="utf-8")
        snapshot_path.write_text(json.dumps(snapshot), encoding="utf-8")

        # The real server starts its poller immediately. Route that one external
        # HTTPS request to a closed local port so the test never depends on Apple.
        server_command = " ".join(
            [
                "env",
                "HTTPS_PROXY=http://127.0.0.1:1",
                "HTTP_PROXY=http://127.0.0.1:1",
                "NO_PROXY=127.0.0.1,localhost",
                shlex.quote(str(SERVER)),
                "-config",
                shlex.quote(str(config_path)),
                "-web-dir",
                shlex.quote(str(FRONTEND.parent)),
            ]
        )
        command = [
            sys.executable,
            str(WITH_SERVER),
            "--server",
            server_command,
            "--port",
            str(port),
            "--timeout",
            "30",
            "--",
            sys.executable,
            str(ROOT / "e2e" / "test_reviews_viewer.py"),
            f"http://127.0.0.1:{port}",
            submitted_at,
            older_submitted_at,
        ]
        completed = subprocess.run(command, cwd=ROOT, env=os.environ.copy(), check=False)
        return completed.returncode


if __name__ == "__main__":
    raise SystemExit(main())
