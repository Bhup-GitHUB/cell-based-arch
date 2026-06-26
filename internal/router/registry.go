package router

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Cell struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Healthy bool   `json:"healthy"`
}

type Registry struct {
	mu       sync.RWMutex
	cells    []Cell
	interval time.Duration
}

func NewRegistry(cells []Cell, pollInterval time.Duration) *Registry {
	return &Registry{
		cells:    cells,
		interval: pollInterval,
	}
}

func (r *Registry) Cells() []Cell {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Cell, len(r.cells))
	copy(out, r.cells)
	return out
}

func (r *Registry) Pick(customerID string) (Cell, error) {
	return Pick(customerID, r.Cells())
}

func (r *Registry) setHealthy(id string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.cells {
		if r.cells[i].ID == id {
			r.cells[i].Healthy = healthy
			return
		}
	}
}

func (r *Registry) StartPoller(ctx context.Context) {
	go r.poll(ctx)
}

func (r *Registry) poll(ctx context.Context) {
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.checkAll(ctx, client)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkAll(ctx, client)
		}
	}
}

func (r *Registry) checkAll(ctx context.Context, client *http.Client) {
	cells := r.Cells()
	for _, c := range cells {
		go func(cell Cell) {
			url := cell.URL + "/healthz"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				r.setHealthy(cell.ID, false)
				slog.Warn("health check request error", "cell", cell.ID, "err", err)
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				r.setHealthy(cell.ID, false)
				slog.Warn("health check failed", "cell", cell.ID, "err", err)
				return
			}
			resp.Body.Close()
			healthy := resp.StatusCode == http.StatusOK
			r.setHealthy(cell.ID, healthy)
			slog.Debug("health check", "cell", cell.ID, "healthy", healthy, "status", resp.StatusCode)
		}(c)
	}
}
