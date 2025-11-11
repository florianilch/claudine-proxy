package app

import (
	"sync/atomic"

	"github.com/florianilch/claudine-proxy/internal/proxy"
)

// Health manages the application's health status for health check endpoints.
// All methods are thread-safe.
type Health struct {
	ready atomic.Bool
}

// Compile-time check that Health implements proxy.ReadinessChecker interface
var _ proxy.ReadinessChecker = (*Health)(nil)

// NewHealth creates a new Health instance initialized as not ready.
func NewHealth() *Health {
	return &Health{}
}

// SetReady updates the application's readiness state.
func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

// IsReady returns the current readiness state of the application.
func (h *Health) IsReady() bool {
	return h.ready.Load()
}
