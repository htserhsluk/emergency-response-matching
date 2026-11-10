//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each resource test owns a schema, so spatial/list assertions cannot see prior runs.
func resourceFixture(t *testing.T) (*storage.Store, *pgxpool.Pool) {
	t.Helper()
	_, admin := fixture(t)
	ctx := context.Background()
	schema := fmt.Sprintf("fixture_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(os.Getenv("ERM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	pool, err := storage.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, file := range []string{"00001_core.sql", "00002_resources.sql", "00003_assignment.sql", "00004_delivery.sql"} {
		b, err := os.ReadFile("../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, strings.Split(string(b), "-- +goose Down")[0]); err != nil {
			t.Fatal(err)
		}
	}
	return storage.New(pool), pool
}

func resourceBody(lat, lon float64, available bool) contracts.RegisterResource {
	return contracts.RegisterResource{Type: "WATER", Location: &contracts.Location{Latitude: &lat, Longitude: &lon}, Availability: &available}
}

func TestResourceTransitions(t *testing.T) {
	s, p := resourceFixture(t)
	ctx := context.Background()
	key := fmt.Sprintf("resource-%d", time.Now().UnixNano())
	a, err := s.RegisterResource(ctx, key, resourceBody(0, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "UNAVAILABLE" || a.Availability {
		t.Fatalf("false ignored: %+v", a)
	}
	b, err := s.RegisterResource(ctx, key, resourceBody(0, 0, false))
	if err != nil || a.ID != b.ID {
		t.Fatalf("replay: %+v %v", b, err)
	}
	if _, err = s.RegisterResource(ctx, key, resourceBody(1, 0, false)); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
	a, err = s.SetResourceStatus(ctx, a.ID, key+"-available", "AVAILABLE")
	if err != nil || !a.Availability {
		t.Fatalf("toggle: %+v %v", a, err)
	}
	if _, err = s.SetResourceStatus(ctx, a.ID, key+"-allocate", "ALLOCATED"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("allocation bypass: %v", err)
	}
	_, err = p.Exec(ctx, "UPDATE resources SET status='ALLOCATED',availability=false,assigned_request_id='00000000000000000000000000000001' WHERE id=$1", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetResourceStatus(ctx, a.ID, key+"-release", "AVAILABLE"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("release bypass: %v", err)
	}
}

func TestResourceSpatialSearchAndPagination(t *testing.T) {
	s, _ := resourceFixture(t)
	ctx := context.Background()
	key := fmt.Sprintf("spatial-%d", time.Now().UnixNano())
	near, err := s.RegisterResource(ctx, key+"-near", resourceBody(62, 41, true))
	if err != nil {
		t.Fatal(err)
	}
	far, err := s.RegisterResource(ctx, key+"-far", resourceBody(62.1, 41, true))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RegisterResource(ctx, key+"-unavailable", resourceBody(62, 41, false))
	if err != nil {
		t.Fatal(err)
	}
	query := contracts.SearchQuery{Type: "WATER", Location: resourceBody(62, 41, true).Location, RadiusMeters: 20000, Limit: 100}
	candidates, err := s.Search(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].ResourceID != near.ID || candidates[0].DistanceMeters != 0 || candidates[1].ResourceID != far.ID || candidates[1].DistanceMeters < 10000 {
		t.Fatalf("spatial: %+v", candidates)
	}
	query.AfterID = near.ID
	query.AfterDistance = 0
	candidates, err = s.Search(ctx, query)
	if err != nil || len(candidates) != 1 || candidates[0].ResourceID != far.ID {
		t.Fatalf("candidate cursor: %+v %v", candidates, err)
	}
	page, err := s.ListResources(ctx, contracts.ListQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page: %+v %v", page, err)
	}
	next, err := s.ListResources(ctx, contracts.ListQuery{Limit: 100, After: page.Next})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range next.Items {
		if r.ID <= page.Next {
			t.Fatal("cursor repeated item")
		}
	}
}

func TestCrossShardNearestAndIncompleteSearch(t *testing.T) {
	const secret = "integration-resource-secret-32-chars"
	var urls [3]string
	var servers [3]*httptest.Server
	for i := range urls {
		s, _ := resourceFixture(t)
		servers[i] = httptest.NewServer(httpapp.NewServer(s, secret, true))
		t.Cleanup(servers[i].Close)
		urls[i] = servers[i].URL
	}
	router := coordinator.NewRouter(urls, secret)
	coord := httptest.NewServer(httpapp.NewServer(router, secret, true))
	defer coord.Close()
	api := httptest.NewServer(httpapp.NewServer(httpapp.NewClient(coord.URL, secret), "", false))
	defer api.Close()
	client := httpapp.NewPublicClient(api.URL)
	ctx := context.Background()
	ids := map[int]string{}
	for shard := range 3 {
		for n := 0; ; n++ {
			key := fmt.Sprintf("cross-shard-resource-%d-%d", shard, n)
			s, _ := routing.ShardFor(contracts.EntityID("register-resource", key))
			if s != shard {
				continue
			}
			r, err := client.RegisterResource(ctx, key, resourceBody(float64(3-shard)/100, 0, true))
			if err != nil {
				t.Fatal(err)
			}
			ids[shard] = r.ID
			break
		}
	}
	query := contracts.SearchQuery{Type: "WATER", Location: resourceBody(0, 0, true).Location, RadiusMeters: 10000, Limit: 3}
	got, err := router.Search(ctx, query)
	if err != nil || len(got) != 3 || got[0].ResourceID != ids[2] || got[2].ResourceID != ids[0] {
		t.Fatalf("global nearest: %+v %v", got, err)
	}
	page, err := client.ListResources(ctx, contracts.ListQuery{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.Items[0].ID >= page.Items[1].ID {
		t.Fatalf("merged page: %+v %v", page, err)
	}
	next, err := client.ListResources(ctx, contracts.ListQuery{Limit: 2, After: page.Next})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID <= page.Next {
		t.Fatalf("next page: %+v %v", next, err)
	}
	servers[1].Close()
	if _, err = router.Search(ctx, query); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("partial search reported complete: %v", err)
	}
	if _, err = client.ListResources(ctx, contracts.ListQuery{Limit: 2}); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("partial list reported complete: %v", err)
	}
}
