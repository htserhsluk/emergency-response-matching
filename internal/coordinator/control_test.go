package coordinator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestControlPersistsPromotionAndRefusesUnsafeActivation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	if _, err := OpenControl(path); err == nil {
		t.Fatal("missing state silently initialized")
	}
	if err := InitializeControl(path); err != nil {
		t.Fatal(err)
	}
	if err := InitializeControl(path); err == nil {
		t.Fatal("existing state overwritten")
	}
	c, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	reg := NewRegistry(func() time.Time { return now })
	reg.Register(NodeReport{NodeID: "node-0a", Shard: 0, BootID: "a", Sequence: 1, Reachable: true, Primary: true})
	reg.Register(NodeReport{NodeID: "node-0b", Shard: 0, BootID: "b", Sequence: 1, Reachable: true, Primary: true})
	if err = c.Activate(0, "node-0b", 1, reg); err == nil {
		t.Fatal("live old primary replaced")
	}
	now = now.Add(9 * time.Second)
	reg.Heartbeat(NodeReport{NodeID: "node-0b", Shard: 0, BootID: "b", Sequence: 2, Reachable: true, Primary: true})
	if err = c.Activate(0, "node-1b", 1, reg); err == nil {
		t.Fatal("wrong shard activated")
	}
	if err = c.Activate(0, "node-0b", 1, reg); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	route := restored.Route(0)
	if route.NodeID != "node-0b" || route.Version != 2 {
		t.Fatalf("lost promotion: %+v", route)
	}
	if err = restored.Activate(0, "node-0a", 1, reg); err == nil {
		t.Fatal("stale operator version accepted")
	}
	if err = os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenControl(path); err == nil {
		t.Fatal("invalid state reset")
	}
}
