package router

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func NewMux(reg *Registry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/orders", handlePostOrder(reg))
	mux.HandleFunc("GET /v1/orders/{customer_id}", handleGetOrders(reg))
	mux.HandleFunc("GET /whereis/{customer_id}", handleWhereis(reg))
	mux.HandleFunc("GET /cells", handleCells(reg))
	mux.HandleFunc("GET /healthz", handleHealthz())
	return mux
}

func handlePostOrder(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read body"})
			return
		}

		var payload struct {
			CustomerID string `json:"customer_id"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.CustomerID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing customer_id"})
			return
		}

		cell, err := reg.Pick(payload.CustomerID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		if !cell.Healthy {
			slog.Warn("cell unavailable for POST /v1/orders", "cell", cell.ID, "customer_id", payload.CustomerID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cell unavailable", "cell": cell.ID})
			return
		}

		w.Header().Set("X-Cell-Id", cell.ID)
		targetURL := cell.URL + "/v1/orders"
		slog.Info("forwarding POST /v1/orders", "cell", cell.ID, "customer_id", payload.CustomerID)
		forward(w, r, targetURL, body)
	}
}

func handleGetOrders(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID := r.PathValue("customer_id")

		cell, err := reg.Pick(customerID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		if !cell.Healthy {
			slog.Warn("cell unavailable for GET /v1/orders", "cell", cell.ID, "customer_id", customerID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cell unavailable", "cell": cell.ID})
			return
		}

		w.Header().Set("X-Cell-Id", cell.ID)
		targetURL := fmt.Sprintf("%s/v1/orders/%s", cell.URL, customerID)
		slog.Info("forwarding GET /v1/orders", "cell", cell.ID, "customer_id", customerID)
		forward(w, r, targetURL, nil)
	}
}

func handleWhereis(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID := r.PathValue("customer_id")

		cell, err := reg.Pick(customerID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"customer_id": customerID,
			"cell":        cell.ID,
			"cell_url":    cell.URL,
		})
	}
}

func handleCells(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, reg.Cells())
	}
}

func handleHealthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "err", err)
	}
}
