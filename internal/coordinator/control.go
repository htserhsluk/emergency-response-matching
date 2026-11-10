package coordinator

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Route struct {
	NodeID  string `json:"nodeId"`
	Version uint64 `json:"version"`
}
type Control struct {
	mu       sync.RWMutex
	path     string
	routes   [3]Route
	disabled bool
}

func InitializeControl(path string) error {
	var routes [3]Route
	for i := range routes {
		routes[i] = Route{fmt.Sprintf("node-%da", i), 1}
	}
	data, _ := json.Marshal(routes)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
func validNode(shard int, id string) bool {
	return id == fmt.Sprintf("node-%da", shard) || id == fmt.Sprintf("node-%db", shard)
}
func OpenControl(path string) (*Control, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Control{path: path}
	if err = json.Unmarshal(data, &c.routes); err != nil {
		return nil, err
	}
	for i, r := range c.routes {
		if !validNode(i, r.NodeID) || r.Version == 0 {
			return nil, errors.New("invalid route control state")
		}
	}
	return c, nil
}
func (c *Control) Route(shard int) Route {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if shard < 0 || shard > 2 || c.disabled {
		return Route{}
	}
	return c.routes[shard]
}

// Activation is an operator action AFTER fencing. Heartbeat loss alone never promotes.
func (c *Control) Activate(shard int, id string, expected uint64, registry *Registry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled || shard < 0 || shard > 2 || !validNode(shard, id) {
		return errors.New("invalid activation")
	}
	old := c.routes[shard]
	if old.Version != expected || old.NodeID == id || registry.Writable(old.NodeID) || !registry.Writable(id) {
		return errors.New("unsafe or stale activation")
	}
	routes := c.routes
	routes[shard] = Route{id, old.Version + 1}
	data, _ := json.Marshal(routes)
	f, err := os.CreateTemp(filepath.Dir(c.path), ".routes-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(temp, c.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(c.path))
	if err == nil {
		err = dir.Sync()
		dir.Close()
	}
	if err != nil {
		c.disabled = true
		return err
	}
	c.routes = routes
	return nil
}
func (c *Control) Handler(next http.Handler, registry *Registry, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/routes" && !strings.HasPrefix(r.URL.Path, "/internal/routes/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(r.Header.Get("X-Internal-Secret"))) != 1 {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"code": "UNAUTHORIZED"})
			return
		}
		if r.Method == "GET" && r.URL.Path == "/internal/routes" {
			routes := [3]Route{}
			for i := range routes {
				routes[i] = c.Route(i)
			}
			json.NewEncoder(w).Encode(routes)
			return
		}
		shard, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/internal/routes/"))
		var v struct {
			NodeID          string `json:"nodeId"`
			ExpectedVersion uint64 `json:"expectedVersion"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.Method != "PUT" || err != nil || json.NewDecoder(r.Body).Decode(&v) != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"code": "INVALID"})
			return
		}
		if err = c.Activate(shard, v.NodeID, v.ExpectedVersion, registry); err != nil {
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]string{"code": "UNSAFE_ACTIVATION"})
			return
		}
		json.NewEncoder(w).Encode(c.Route(shard))
	})
}
