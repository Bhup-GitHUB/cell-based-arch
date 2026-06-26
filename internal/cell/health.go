package cell

import "sync/atomic"

type Health struct {
	faulted atomic.Bool
}

func NewHealth(startFaulted bool) *Health {
	h := &Health{}
	h.faulted.Store(startFaulted)
	return h
}

func (h *Health) SetFault(v bool) {
	h.faulted.Store(v)
}

func (h *Health) Faulted() bool {
	return h.faulted.Load()
}
