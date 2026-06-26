package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Bhup-GitHUB/cell-based-arch/internal/router"
)

func main() {
	cells, err := parseCells(env("CELLS", ""))
	if err != nil {
		slog.Error("invalid CELLS env", "err", err)
		os.Exit(1)
	}

	pollInterval, err := time.ParseDuration(env("HEALTH_POLL_INTERVAL", "2s"))
	if err != nil {
		slog.Error("invalid HEALTH_POLL_INTERVAL", "err", err)
		os.Exit(1)
	}

	port := env("PORT", "8080")

	reg := router.NewRegistry(cells, pollInterval)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg.StartPoller(ctx)

	mux := router.NewMux(reg)
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		slog.Info("router listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseCells(raw string) ([]router.Cell, error) {
	if raw == "" {
		return nil, fmt.Errorf("CELLS must not be empty")
	}
	pairs := strings.Split(raw, ",")
	cells := make([]router.Cell, 0, len(pairs))
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		idx := strings.IndexByte(pair, '=')
		if idx < 1 {
			return nil, fmt.Errorf("invalid cell spec %q: expected id=url", pair)
		}
		id := strings.TrimSpace(pair[:idx])
		url := strings.TrimSpace(pair[idx+1:])
		if id == "" || url == "" {
			return nil, fmt.Errorf("invalid cell spec %q: id and url must be non-empty", pair)
		}
		cells = append(cells, router.Cell{ID: id, URL: url, Healthy: false})
	}
	return cells, nil
}
