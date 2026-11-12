package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMapConfigAndPinnedAssets(t *testing.T) {
	t.Setenv("ERM_MAP_TILE_URL", "")
	h := Handler(http.NotFoundHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/web-config", nil))
	var c map[string]string
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &c) != nil || c["tileUrl"] != "" {
		t.Fatalf("disabled map config %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/map.js", "/vendor/leaflet/leaflet.js", "/vendor/leaflet/leaflet.css"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("missing pinned map asset %s", path)
		}
	}
}

func TestBrowserAssetsAndAPIFallback(t *testing.T) {
	h := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(418) }))
	for _, path := range []string{"/", "/app.js", "/styles.css"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("asset %s: %d", path, w.Code)
		}
		if path == "/" && !strings.Contains(w.Body.String(), "Request status") {
			t.Fatal("missing usable request workflow")
		}
	}
	for _, path := range []string{"/requests/abc", "/internal/routes", "/unknown"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 418 {
			t.Fatalf("intercepted API %s", path)
		}
	}
}
