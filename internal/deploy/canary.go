package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func RunCanary(ctx context.Context, service, healthURL string, consecutiveRequired int, perRequestTimeout time.Duration) error {
	client := &http.Client{Timeout: perRequestTimeout}
	consecutive := 0
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("canary timed out for %s after %d attempts (%d consecutive successes, need %d)", service, attempt, consecutive, consecutiveRequired)
		default:
		}

		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			consecutive = 0
			slog.Warn("canary: request build failed", "service", service, "attempt", attempt, "err", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			consecutive = 0
			slog.Warn("canary: probe failed", "service", service, "attempt", attempt, "err", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			consecutive++
			slog.Info("canary: probe ok", "service", service, "attempt", attempt, "consecutive", consecutive, "need", consecutiveRequired)
			if consecutive >= consecutiveRequired {
				slog.Info("canary: passed", "service", service, "consecutive_successes", consecutive)
				return nil
			}
		} else {
			consecutive = 0
			slog.Warn("canary: probe non-200", "service", service, "attempt", attempt, "status", resp.StatusCode)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
