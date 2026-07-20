SHELL := /bin/bash

GO ?= go
NPM ?= npm
PYTHON ?= python3
BACKEND_DEV_CMD ?= $(GO) run ./cmd/server
FRONTEND_DEV_CMD ?= $(NPM) --prefix web run dev

.PHONY: setup setup-e2e dev dev-backend dev-frontend test test-backend test-frontend test-frontend-coverage test-e2e vet build build-backend build-frontend run clean

setup:
	$(NPM) --prefix web ci

setup-e2e:
	$(PYTHON) -m pip install -r e2e/requirements.txt
	$(PYTHON) -m playwright install chromium

# Run both development servers. If either one exits, stop the other and fail
# without requiring a process-runner package.
dev:
	@set -u; \
	$(BACKEND_DEV_CMD) & backend_pid=$$!; \
	$(FRONTEND_DEV_CMD) & frontend_pid=$$!; \
	cleanup() { \
		trap - EXIT INT TERM; \
		kill "$$backend_pid" "$$frontend_pid" 2>/dev/null || true; \
		wait "$$backend_pid" "$$frontend_pid" 2>/dev/null || true; \
	}; \
	trap cleanup EXIT INT TERM; \
	while :; do \
		for pid in "$$backend_pid" "$$frontend_pid"; do \
			if ! kill -0 "$$pid" 2>/dev/null; then \
				wait "$$pid"; status=$$?; \
				exit "$$status"; \
			fi; \
		done; \
		sleep 0.1; \
	done

dev-backend:
	$(GO) run ./cmd/server

dev-frontend:
	$(NPM) --prefix web run dev

test: test-backend test-frontend

test-backend:
	$(GO) test -race ./...

test-frontend:
	$(NPM) --prefix web test

test-frontend-coverage:
	$(NPM) --prefix web run test:coverage

# Builds the production binary/UI, then tests the real server and API in Chromium
# against an isolated persisted snapshot. No live Apple request is required.
test-e2e: build
	$(PYTHON) e2e/run.py

vet:
	$(GO) vet ./...

build: build-backend build-frontend

build-backend:
	mkdir -p bin
	$(GO) build -o bin/reviews-viewer ./cmd/server

build-frontend:
	$(NPM) --prefix web run build

# The production-style Go server serves both the API and web/dist.
run: build
	./bin/reviews-viewer

clean:
	rm -rf bin web/dist web/coverage
