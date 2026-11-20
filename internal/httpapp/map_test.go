package httpapp

import (
	"net/http/httptest"
	"testing"
)

func TestMapInputValidation(t *testing.T) {
	h := NewServer(nil, "", false)
	for _, path := range []string{"/map/locations?type=UNKNOWN", "/map/locations?type=WATER&after=invalid", "/resources/nope/candidates"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Errorf("%s: got %d, want 400", path, w.Code)
		}
	}
}
