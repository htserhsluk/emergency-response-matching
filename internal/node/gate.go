package node

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Activation is learned only from the authenticated coordinator heartbeat response.
// A request header can present a version but can never grant authority.
type Gate struct {
	mu      sync.RWMutex
	version uint64
	until   time.Time
	primary func(context.Context) bool
	now     func() time.Time
}

func NewGate(primary func(context.Context) bool, now func() time.Time) *Gate {
	return &Gate{primary: primary, now: now}
}
func (g *Gate) Activate(v uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.version = v
	g.until = g.now().Add(4 * time.Second)
}
func (g *Gate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path != "/health/ready" {
			next.ServeHTTP(w, r)
			return
		}
		version, err := strconv.ParseUint(r.Header.Get("X-Route-Version"), 10, 64)
		g.mu.RLock()
		if r.URL.Path == "/health/ready" {
			version = g.version
			err = nil
		}
		allowed := err == nil && version != 0 && version == g.version && g.now().Before(g.until)
		g.mu.RUnlock()
		probe, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if !allowed || !g.primary(probe) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]string{"code": "UNAVAILABLE", "message": "Node is not activated for this route version"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
