package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunPropagatesStartupError(t *testing.T) {
	err := run(filepath.Join(t.TempDir(), "missing.json"), "", testLogger())
	if err == nil || !strings.Contains(err.Error(), "load configuration") {
		t.Fatalf("run returned %v, want configuration error", err)
	}
}

func TestFrontendDirAndServingAreIndependentOfWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "app.json")
	webDir := filepath.Join(root, "web", "dist")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	dir := frontendDir(configPath, "")
	if dir != webDir {
		t.Fatalf("frontendDir = %q, want %q", dir, webDir)
	}
	mux := http.NewServeMux()
	registerFrontend(mux, dir)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
	if response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("frontend response = %d %q, want 200 ok", response.Code, response.Body.String())
	}
}

func TestRunServerPropagatesServeErrorAndWaitsForPoller(t *testing.T) {
	serveErr := errors.New("listener failed")
	shutdownCalled := false
	server := &fakeLifecycleServer{
		serve: func(net.Listener) error { return serveErr },
		shutdown: func(context.Context) error {
			shutdownCalled = true
			return nil
		},
	}
	pollerStopped := make(chan struct{})

	err := runServer(
		context.Background(),
		server,
		stubListener{},
		func(ctx context.Context) {
			<-ctx.Done()
			close(pollerStopped)
		},
		testLogger(),
		time.Second,
	)

	if !errors.Is(err, serveErr) {
		t.Fatalf("runServer returned %v, want wrapped serve error", err)
	}
	if !shutdownCalled {
		t.Fatal("HTTP shutdown was not attempted after serve failure")
	}
	select {
	case <-pollerStopped:
	default:
		t.Fatal("runServer returned before the poller stopped")
	}
}

func TestRunServerGracefulCancellationWaitsForPoller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopServe := make(chan struct{})
	serveReturned := make(chan struct{})
	server := &fakeLifecycleServer{
		serve: func(net.Listener) error {
			defer close(serveReturned)
			<-stopServe
			return http.ErrServerClosed
		},
		shutdown: func(context.Context) error {
			close(stopServe)
			return nil
		},
	}
	pollerStopped := make(chan struct{})

	err := runServer(
		ctx,
		server,
		stubListener{},
		func(ctx context.Context) {
			<-ctx.Done()
			close(pollerStopped)
		},
		testLogger(),
		time.Second,
	)

	if err != nil {
		t.Fatalf("graceful shutdown returned error: %v", err)
	}
	select {
	case <-pollerStopped:
	default:
		t.Fatal("runServer returned before the poller stopped")
	}
	select {
	case <-serveReturned:
	default:
		t.Fatal("runServer returned before the HTTP server goroutine stopped")
	}
}

func TestRunServerPropagatesShutdownAndCloseErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownErr := errors.New("shutdown timed out")
	closeErr := errors.New("close failed")
	serveStopped := make(chan struct{})
	server := &fakeLifecycleServer{
		serve: func(net.Listener) error {
			<-serveStopped
			return http.ErrServerClosed
		},
		shutdown: func(context.Context) error { return shutdownErr },
		close: func() error {
			close(serveStopped)
			return closeErr
		},
	}

	err := runServer(
		ctx,
		server,
		stubListener{},
		func(ctx context.Context) { <-ctx.Done() },
		testLogger(),
		time.Second,
	)

	if !errors.Is(err, shutdownErr) || !errors.Is(err, closeErr) {
		t.Fatalf("runServer returned %v, want joined shutdown and close errors", err)
	}
}

type fakeLifecycleServer struct {
	serve    func(net.Listener) error
	shutdown func(context.Context) error
	close    func() error
}

func (s *fakeLifecycleServer) Serve(listener net.Listener) error {
	return s.serve(listener)
}

func (s *fakeLifecycleServer) Shutdown(ctx context.Context) error {
	return s.shutdown(ctx)
}

func (s *fakeLifecycleServer) Close() error {
	if s.close == nil {
		return nil
	}
	return s.close()
}

type stubListener struct{}

func (stubListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (stubListener) Close() error              { return nil }
func (stubListener) Addr() net.Addr            { return stubAddr("test") }

type stubAddr string

func (a stubAddr) Network() string { return string(a) }
func (a stubAddr) String() string  { return string(a) }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
