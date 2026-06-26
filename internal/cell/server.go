package cell

import (
	"context"
	"encoding/json"
	"net/http"
)

type Server struct {
	cellID  string
	version string
	store   *Store
	health  *Health
	mux     *http.ServeMux
}

func NewServer(cellID, version string, store *Store, health *Health) *Server {
	s := &Server{
		cellID:  cellID,
		version: version,
		store:   store,
		health:  health,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /v1/orders", s.handleCreateOrder)
	s.mux.HandleFunc("GET /v1/orders/{customer_id}", s.handleListOrders)
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	s.mux.HandleFunc("POST /admin/fault", s.handleFault)
}

func (s *Server) cellHeader(w http.ResponseWriter) {
	w.Header().Set("X-Cell-Id", s.cellID)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	s.cellHeader(w)

	if s.health.Faulted() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cell is faulted"})
		return
	}

	var body struct {
		CustomerID string  `json:"customer_id"`
		Item       string  `json:"item"`
		Amount     float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CustomerID == "" || body.Item == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	order, err := s.store.CreateOrder(r.Context(), body.CustomerID, body.Item, body.Amount)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.writeJSON(w, http.StatusCreated, order)
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	s.cellHeader(w)

	customerID := r.PathValue("customer_id")
	orders, err := s.store.OrdersByCustomer(r.Context(), customerID)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if orders == nil {
		orders = []Order{}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"cell":        s.cellID,
		"customer_id": customerID,
		"orders":      orders,
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	s.cellHeader(w)

	if s.health.Faulted() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":  "unhealthy",
			"cell":    s.cellID,
			"version": s.version,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*pingTimeout)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":  "unhealthy",
			"cell":    s.cellID,
			"version": s.version,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"cell":    s.cellID,
		"version": s.version,
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	s.cellHeader(w)

	ctx, cancel := context.WithTimeout(r.Context(), 2*pingTimeout)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFault(w http.ResponseWriter, r *http.Request) {
	s.cellHeader(w)

	var body struct {
		Fail bool `json:"fail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	s.health.SetFault(body.Fail)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"cell":  s.cellID,
		"fault": body.Fail,
	})
}
