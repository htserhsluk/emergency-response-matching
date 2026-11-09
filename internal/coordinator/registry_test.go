package coordinator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRetiredBootCannotRegisterAgain(t *testing.T) {
	r := NewRegistry(time.Now)
	a := NodeReport{NodeID: "node-0a", Shard: 0, BootID: "a", Sequence: 1, Reachable: true, Primary: true}
	if err := r.Register(a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.BootID = "b"
	b.Reachable = false
	if err := r.Register(b); err != nil {
		t.Fatal(err)
	}
	a.Sequence = 2
	if err := r.Register(a); err == nil {
		t.Fatal("retired boot replaced current observation")
	}
	if r.Writable("node-0a") {
		t.Fatal("retired primary revived routing")
	}
}

func TestReadinessRequiresEligibleRegisteredRoute(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer endpoint.Close()
	now := time.Now()
	registry := NewRegistry(func() time.Time { return now })
	router := NewRouter([3]string{endpoint.URL}, "secret")
	router.Registry = registry
	router.NodeIDs[0] = "node-0a"
	if router.Health(context.Background()) == nil {
		t.Fatal("unregistered route marked ready")
	}
	n := NodeReport{NodeID: "node-0a", Shard: 0, BootID: "boot", Sequence: 1, Reachable: true, Primary: true}
	registry.Register(n)
	if err := router.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(9 * time.Second)
	if router.Health(context.Background()) == nil {
		t.Fatal("expired route marked ready")
	}
	n.Sequence = 2
	n.Primary = false
	registry.Heartbeat(n)
	if router.Health(context.Background()) == nil {
		t.Fatal("standby marked writable-ready")
	}
}

func TestHeartbeatExpiryAndDatabaseRole(t *testing.T) {
	now := time.Unix(100, 0)
	r := NewRegistry(func() time.Time { return now })
	report := NodeReport{NodeID: "node-0a", Shard: 0, BootID: "boot-one", Sequence: 1, Reachable: true, Primary: true}
	if err := r.Register(report); err != nil {
		t.Fatal(err)
	}
	if !r.Writable("node-0a") {
		t.Fatal("healthy primary excluded")
	}
	now = now.Add(9 * time.Second)
	if r.Writable("node-0a") {
		t.Fatal("expired node still writable")
	}
	report.Sequence = 2
	report.Reachable = false
	r.Heartbeat(report)
	if r.Writable("node-0a") {
		t.Fatal("broken DB considered healthy")
	}
	report.Sequence = 3
	report.Reachable = true
	report.Primary = false
	r.Heartbeat(report)
	if r.Writable("node-0a") {
		t.Fatal("standby considered writable")
	}
	report.Sequence = 2
	report.Primary = true
	if err := r.Heartbeat(report); err == nil {
		t.Fatal("out-of-order heartbeat accepted")
	}
	report.Sequence = 4
	report.BootID = "old-boot"
	if err := r.Heartbeat(report); err == nil {
		t.Fatal("unregistered boot accepted")
	}
}
