package httpapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidRequestJSON(t *testing.T) {
	h := NewServer(nil, "", false)
	for _, body := range []string{
		`{}`, `null`, `{"type":"WATER"} {}`, `{"extra":1}`, `{"type":"WATER","location":{"latitude":0,"longitude":0},"details":"need","unexpected":true}`,
	} {
		r := httptest.NewRequest("POST", "/requests", strings.NewReader(body))
		r.Header.Set("Idempotency-Key", "request-key-00001")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Errorf("%s: got %d", body, w.Code)
		}
	}
}

func TestInternalAccessDeniedBeforeStorage(t *testing.T) {
	h := NewServer(nil, "long-enough-internal-secret", true)
	r := httptest.NewRequest("GET", "/internal/requests/00000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", w.Code)
	}
}

func TestOversizeRequestAndJSONErrors(t *testing.T) {
	h := NewServer(nil, "", false)
	r := httptest.NewRequest("POST", "/requests", strings.NewReader(strings.Repeat(" ", 65537)))
	r.Header.Set("Idempotency-Key", "request-key-00001")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatalf("got %d", w.Code)
	}
	for _, method := range []string{"DELETE", "PATCH"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/requests", nil))
		if w.Code != 405 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("non JSON method error %d", w.Code)
		}
	}
}
