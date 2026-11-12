package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

//go:embed index.html app.js styles.css map.js vendor
var assets embed.FS

func Handler(api http.Handler) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/web-config" && r.Method == "GET" {
			url, exists := os.LookupEnv("ERM_MAP_TILE_URL")
			if !exists {
				url = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
			}
			attribution := os.Getenv("ERM_MAP_ATTRIBUTION")
			if attribution == "" {
				attribution = "© OpenStreetMap contributors"
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"tileUrl": url, "attribution": attribution})
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/app.js" || r.URL.Path == "/styles.css" || r.URL.Path == "/map.js" || strings.HasPrefix(r.URL.Path, "/vendor/leaflet/") {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}
