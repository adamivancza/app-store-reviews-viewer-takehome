// Command server runs the App Store reviews HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/adamivancza/recent-ios-reviews-viewer/internal/reviews"
)

func main() {
	configPath := flag.String("config", "config/app.json", "path to app configuration")
	webDir := flag.String("web-dir", "", "compiled frontend directory (default: ../web/dist relative to the config file)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(*configPath, *webDir, logger); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run(configPath, webDir string, logger *slog.Logger) error {
	var err error
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve configuration path: %w", err)
	}
	app, err := reviews.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	store := reviews.NewJSONStore(app.DataDir)
	feed := reviews.NewAppleFeedClient(&http.Client{Timeout: 15 * time.Second})
	poller, err := reviews.NewPoller(app, store, feed, logger)
	if err != nil {
		return fmt.Errorf("initialize poller: %w", err)
	}

	mux := http.NewServeMux()
	reviews.NewAPI(poller).Register(mux)
	registerFrontend(mux, frontendDir(configPath, webDir))
	server := &http.Server{
		Addr: app.ListenAddr, Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	listener, err := net.Listen("tcp", app.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", app.ListenAddr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("server listening", "address", listener.Addr().String(), "app", app.Key)
	return runServer(ctx, server, listener, poller.Run, logger, 10*time.Second)
}

func frontendDir(configPath, webDir string) string {
	if webDir == "" {
		webDir = filepath.Join("..", "web", "dist")
	}
	if filepath.IsAbs(webDir) {
		return webDir
	}
	return filepath.Join(filepath.Dir(configPath), webDir)
}

type lifecycleServer interface {
	Serve(net.Listener) error
	Shutdown(context.Context) error
	Close() error
}

func runServer(
	parent context.Context,
	server lifecycleServer,
	listener net.Listener,
	runPoller func(context.Context),
	logger *slog.Logger,
	shutdownTimeout time.Duration,
) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var pollerWG sync.WaitGroup
	pollerWG.Add(1)
	go func() {
		defer pollerWG.Done()
		runPoller(ctx)
	}()

	serverErrors := make(chan error, 1)
	var serverWG sync.WaitGroup
	serverWG.Add(1)
	go func() {
		defer serverWG.Done()
		serverErrors <- server.Serve(listener)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-serverErrors:
		if err == nil {
			runErr = errors.New("HTTP server stopped unexpectedly without an error")
		} else {
			runErr = fmt.Errorf("serve HTTP: %w", err)
		}
		cancel()
	}

	shutdownCtx, stopShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer stopShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shut down HTTP server: %w", err))
		if closeErr := server.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("force close HTTP server: %w", closeErr))
		}
	}
	cancel()
	pollerWG.Wait()
	serverWG.Wait()
	return runErr
}

func registerFrontend(mux *http.ServeMux, dir string) {
	index := filepath.Join(dir, "index.html")
	if info, err := os.Stat(index); err == nil && !info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(dir)))
		return
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}
