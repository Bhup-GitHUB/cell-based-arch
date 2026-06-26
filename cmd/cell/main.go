package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Bhup-GitHUB/cell-based-arch/internal/cell"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cellID := mustEnv("CELL_ID")
	version := envOrDefault("APP_VERSION", "v1")
	dbURL := mustEnv("DATABASE_URL")
	port := envOrDefault("PORT", "9000")
	startFaulted := os.Getenv("CELL_FAIL") == "true"

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := connectWithRetry(ctx, dbURL, 30)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := cell.NewStore(pool)
	if err := store.Bootstrap(ctx); err != nil {
		slog.Error("schema bootstrap failed", "error", err)
		os.Exit(1)
	}

	health := cell.NewHealth(startFaulted)
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: cell.NewServer(cellID, version, store, health),
	}

	go func() {
		slog.Info("cell started", "cell", cellID, "version", version, "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down", "cell", cellID)

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}

func connectWithRetry(ctx context.Context, dsn string, maxAttempts int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			pingErr := pool.Ping(pingCtx)
			cancel()
			if pingErr == nil {
				return pool, nil
			}
			pool.Close()
			slog.Warn("database ping failed, retrying", "attempt", attempt, "max", maxAttempts, "error", pingErr)
		} else {
			slog.Warn("database connect failed, retrying", "attempt", attempt, "max", maxAttempts, "error", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return nil, errors.New("exhausted database connection retries")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required env var not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
