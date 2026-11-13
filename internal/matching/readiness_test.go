package matching

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsumerReadinessDoesNotReplaceLiveness(t *testing.T) {
	s := &SessionState{}
	h := s.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	check := func(path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatalf("%s: %d want %d", path, w.Code, want)
		}
	}
	check("/health/ready", 503)
	check("/health/live", 200)
	s.connected.Store(true)
	check("/health/ready", 200)
	s.connected.Store(false)
	check("/health/ready", 503)
}
