package coordinator

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

type NodeReport struct {
	NodeID      string `json:"nodeId"`
	Shard       int    `json:"shard"`
	BootID      string `json:"bootId"`
	Sequence    uint64 `json:"sequence"`
	Reachable   bool   `json:"reachable"`
	Primary     bool   `json:"primary"`
	WALPosition string `json:"walPosition"`
}
type observation struct {
	Report   NodeReport `json:"report"`
	Received time.Time  `json:"received"`
}
type Registry struct {
	Control *Control
	mu      sync.RWMutex
	now     func() time.Time
	nodes   map[string]observation
	retired map[string]map[string]bool
}

func NewRegistry(now func() time.Time) *Registry {
	return &Registry{now: now, nodes: make(map[string]observation), retired: make(map[string]map[string]bool)}
}
func (r *Registry) Register(n NodeReport) error {
	if n.NodeID == "" || len(n.NodeID) > 64 || n.BootID == "" || len(n.BootID) > 64 || n.Shard < 0 || n.Shard > 2 || n.Sequence == 0 {
		return errors.New("invalid registration")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.retired[n.NodeID][n.BootID] {
		return errors.New("retired boot")
	}
	if old, ok := r.nodes[n.NodeID]; ok && old.Report.BootID == n.BootID && old.Report.Sequence >= n.Sequence {
		return errors.New("stale registration")
	}
	if old, ok := r.nodes[n.NodeID]; ok && old.Report.BootID != n.BootID {
		if r.retired[n.NodeID] == nil {
			r.retired[n.NodeID] = make(map[string]bool)
		}
		r.retired[n.NodeID][old.Report.BootID] = true
	}
	r.nodes[n.NodeID] = observation{n, r.now()}
	return nil
}
func (r *Registry) Heartbeat(n NodeReport) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.nodes[n.NodeID]
	if !ok || old.Report.BootID != n.BootID || old.Report.Shard != n.Shard || old.Report.Sequence >= n.Sequence {
		return errors.New("unregistered or stale heartbeat")
	}
	r.nodes[n.NodeID] = observation{n, r.now()}
	return nil
}
func (r *Registry) Writable(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.nodes[id]
	return ok && o.Report.Reachable && o.Report.Primary && r.now().Sub(o.Received) < 8*time.Second
}
func (r *Registry) Handler(next http.Handler, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path
		if path != "/internal/nodes/register" && path != "/internal/nodes/heartbeat" && path != "/internal/nodes" {
			next.ServeHTTP(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(req.Header.Get("X-Internal-Secret"))) != 1 {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"code": "UNAUTHORIZED"})
			return
		}
		if path == "/internal/nodes" && req.Method == "GET" {
			r.mu.RLock()
			defer r.mu.RUnlock()
			json.NewEncoder(w).Encode(r.nodes)
			return
		}
		if req.Method != "POST" {
			w.WriteHeader(405)
			json.NewEncoder(w).Encode(map[string]string{"code": "METHOD_NOT_ALLOWED"})
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, 4096)
		var report NodeReport
		if json.NewDecoder(req.Body).Decode(&report) != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"code": "INVALID"})
			return
		}
		var err error
		if path == "/internal/nodes/register" {
			err = r.Register(report)
		} else {
			err = r.Heartbeat(report)
		}
		if err != nil {
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]string{"code": "REGISTRATION_REQUIRED"})
			return
		}
		var version uint64
		if r.Control != nil {
			route := r.Control.Route(report.Shard)
			if route.NodeID == report.NodeID && r.Writable(report.NodeID) {
				version = route.Version
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "accepted", "routeVersion": version})
	})
}
