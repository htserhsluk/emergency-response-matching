//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixture(t *testing.T) (*storage.Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("ERM_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("ERM_TEST_DATABASE_URL required: use isolated integration database")
	}
	pool, err := storage.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = pool.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return storage.New(pool), pool
}

func body() contracts.CreateRequest {
	var c contracts.CreateRequest
	json.Unmarshal([]byte(`{"type":"WATER","location":{"latitude":0,"longitude":0},"details":"integration water request"}`), &c)
	return c
}

func TestCreateReplayAndConflict(t *testing.T) {
	store, pool := fixture(t)
	ctx := context.Background()
	key := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	a, err := store.Create(ctx, key, body())
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Create(ctx, key, body())
	if err != nil || a.ID != b.ID || !a.CreatedAt.Equal(b.CreatedAt) {
		t.Fatalf("unstable replay: %+v %+v %v", a, b, err)
	}
	changed := body()
	changed.Details = "changed"
	if _, err = store.Create(ctx, key, changed); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("want conflict got %v", err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1", a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbox count=%d error=%v", count, err)
	}
	if _, err = store.SetStatus(ctx, a.ID, key+"-status", "ASSIGNED"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("assignment bypass %v", err)
	}
	if _, err = store.SetStatus(ctx, a.ID, key+"-noop", "PENDING"); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentCreationSingleRow(t *testing.T) {
	store, _ := fixture(t)
	key := fmt.Sprintf("concurrent-%d", time.Now().UnixNano())
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := store.Create(context.Background(), key, body())
			if err != nil || r.ID != contracts.EntityID("create-request", key) {
				t.Errorf("%+v %v", r, err)
			}
		}()
	}
	wg.Wait()
}

func TestCanceledTransactionReleasesPool(t *testing.T) {
	store, pool := fixture(t)
	key := fmt.Sprintf("cancel-pool-%d", time.Now().UnixNano())
	id := contracts.EntityID("create-request", key)
	blocker, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(context.Background(), "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_, err = store.Create(ctx, key, body())
		cancel()
		if err == nil {
			t.Fatal("blocked operation unexpectedly succeeded")
		}
	}
	if err = blocker.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err = store.Create(ctx, key, body()); err != nil {
		t.Fatalf("pool unavailable after canceled transactions: %v", err)
	}
}

func TestFullHTTPStoragePath(t *testing.T) {
	store, _ := fixture(t)
	secret := "integration-only-secret-32-characters"
	node := httptest.NewServer(httpapp.NewServer(store, secret, true))
	defer node.Close()
	registry := coordinator.NewRegistry(time.Now)
	report := coordinator.NodeReport{NodeID: "fixture-node", Shard: 2, BootID: "fixture-boot", Sequence: 1, Reachable: true, Primary: true}
	if err := registry.Register(report); err != nil {
		t.Fatal(err)
	}
	router := coordinator.NewRouter([3]string{"", "", node.URL}, secret)
	router.Registry = registry
	router.NodeIDs[2] = "fixture-node"
	coordinatorServer := httptest.NewServer(httpapp.NewServer(router, secret, true))
	defer coordinatorServer.Close()
	api := httptest.NewServer(httpapp.NewServer(httpapp.NewClient(coordinatorServer.URL, secret), "", false))
	defer api.Close()
	client := httpapp.NewPublicClient(api.URL)
	key := fmt.Sprintf("http-flow-%d", time.Now().UnixNano())
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("%s-%d", key, i)
		shard, _ := routing.ShardFor(contracts.EntityID("create-request", candidate))
		if shard == 2 {
			key = candidate
			break
		}
	}
	a, err := client.Create(context.Background(), key, body())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Get(context.Background(), a.ID)
	if err != nil || got.ID != a.ID || got.Status != "PENDING" || *got.Location.Latitude != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err = client.Get(context.Background(), "00000000000000000000000000000000"); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("not found=%v", err)
	}
	report.Sequence = 2
	report.Reachable = false
	registry.Heartbeat(report)
	if _, err = client.Get(context.Background(), a.ID); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("heartbeat exclusion=%v", err)
	}
	report.Sequence = 3
	report.Reachable = true
	registry.Heartbeat(report)
	node.Close()
	if _, err = client.Get(context.Background(), a.ID); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("dependency failure=%v", err)
	}
}
