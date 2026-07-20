#!/usr/bin/env python3
"""Start a server, wait for its port, run a command, and always stop it."""

from __future__ import annotations

import argparse
import os
import signal
import socket
import subprocess
import sys
import time


def wait_for_port(port: int, timeout: int) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=1):
                return True
        except OSError:
            time.sleep(0.1)
    return False


def main() -> int:
    parser = argparse.ArgumentParser(description="Run a command while a server is ready")
    parser.add_argument("--server", required=True, help="server shell command")
    parser.add_argument("--port", required=True, type=int, help="server readiness port")
    parser.add_argument("--timeout", default=30, type=int, help="readiness timeout in seconds")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        parser.error("a command is required after --")

    server = subprocess.Popen(args.server, shell=True, start_new_session=True)
    try:
        if not wait_for_port(args.port, args.timeout):
            raise RuntimeError(f"server did not listen on port {args.port} within {args.timeout}s")
        return subprocess.run(command, check=False).returncode
    finally:
        if server.poll() is None:
            os.killpg(server.pid, signal.SIGTERM)
            try:
                server.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(server.pid, signal.SIGKILL)
                server.wait()


if __name__ == "__main__":
    raise SystemExit(main())
