package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMutationRequiresActivationVersionAndPrimary(t *testing.T) {
	now := time.Now()
	primary := true
	gate := NewGate(func(context.Context) bool { return primary }, func() time.Time { return now })
	handler := gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }))
	check := func(version string, want int) {
		t.Helper()
		req := httptest.NewRequest("POST", "/internal/requests", nil)
		req.Header.Set("X-Route-Version", version)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("version=%s status=%d want=%d", version, w.Code, want)
		}
	}
	check("1", 503)
	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest("GET", "/health/ready", nil))
	if ready.Code != 503 {
		t.Fatalf("unactivated node ready: %d", ready.Code)
	}
	gate.Activate(2)
	check("1", 503)
	check("2", 201)
	primary = false
	check("2", 503)
	primary = true
	now = now.Add(5 * time.Second)
	check("2", 503)
	gate.Activate(3)
	check("2", 503)
	check("3", 201)
	gate.Activate(0)
	check("3", 503)
}
