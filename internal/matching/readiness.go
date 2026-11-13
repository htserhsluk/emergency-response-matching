package matching

import (
	"net/http"
	"sync/atomic"
)

type SessionState struct{ connected atomic.Bool }

func (s *SessionState) Ready() bool { return s.connected.Load() }
func (s *SessionState) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/ready" {
			w.Header().Set("Content-Type", "application/json")
			if !s.Ready() {
				w.WriteHeader(503)
				w.Write([]byte(`{"status":"disconnected"}`))
				return
			}
			w.Write([]byte(`{"status":"ready"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
