//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
)

func TestAllocationHistoryGlobalWindow(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	var expected []string
	for i := range 62 {
		shard := i % 3
		resource, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", shard, fmt.Sprintf("history-%d", i)), resourceBody(0, 0, true))
		if err != nil {
			t.Fatal(err)
		}
		cmd := contracts.ReserveCommand{AssignmentID: fmt.Sprintf("%032x", i+1), ResourceID: resource.ID, RequestID: fmt.Sprintf("%032x", i+1)}
		if _, err = l.stores[shard].Reserve(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		var allocated time.Time
		if err = l.pools[shard].QueryRow(ctx, "SELECT allocated_at FROM resources WHERE id=$1", resource.ID).Scan(&allocated); err != nil || allocated.IsZero() {
			t.Fatal(allocated, err)
		}
		// The final resource was registered last week and allocated today.
		day := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC).Add(-time.Duration(61-i) * 24 * time.Hour)
		registered := day.Add(-time.Hour)
		if i == 61 {
			registered = day.AddDate(0, 0, -7)
		}
		if _, err = l.pools[shard].Exec(ctx, `UPDATE resources SET registered_at=$2,allocated_at=$3 WHERE id=$1`, resource.ID, registered, day); err != nil {
			t.Fatal(err)
		}
		if _, err = l.stores[shard].Reserve(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, resource.ID)
	}
	slices.Reverse(expected)
	q := contracts.ListQuery{Status: "ALLOCATED", Sort: "allocated", Limit: 13}
	var ids []string
	for page := 0; page < 8; page++ {
		got, err := l.router.ListResources(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range got.Items {
			ids = append(ids, item.ID)
		}
		if got.Next == "" {
			break
		}
		q.After = got.Next
	}
	q.After = "0001-01-01T00:00:00Z/" + strings.Repeat("f", 32)
	outside, err := l.router.ListResources(ctx, q)
	if err != nil || len(outside.Items) != 0 || outside.Next != "" {
		t.Fatalf("cursor bypassed top 50: %+v %v", outside, err)
	}
	if !slices.Equal(ids, expected[:50]) {
		t.Fatalf("global history pages = %v; want %v", ids, expected[:50])
	}
	all := 0
	for _, p := range l.pools {
		var count int
		if err := p.QueryRow(ctx, "SELECT count(*) FROM resources WHERE status='ALLOCATED'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		all += count
	}
	if all != 62 {
		t.Fatalf("retained allocations=%d", all)
	}
	q.Limit = 100
	q.After = ""
	got, err := l.router.ListResources(ctx, q)
	if err != nil || len(got.Items) != 50 || got.Next != "" {
		t.Fatalf("limit=100 size=%d next=%q error=%v", len(got.Items), got.Next, err)
	}
}

func TestAllocationHistoryRecoversCrossShardConfirmation(t *testing.T) {
	const secret = "history-fixture-for-cross-shard-recovery"
	ctx := context.Background()
	var urls [3]string
	legacy := contracts.EntityID("register-resource", keyOnShard("register-resource", 2, "legacy-history"))
	undated := contracts.EntityID("register-resource", keyOnShard("register-resource", 1, "unknown-history"))
	request := contracts.EntityID("create-request", keyOnShard("create-request", 0, "legacy-history"))
	migration, err := os.ReadFile("../../migrations/00010_allocation_history.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		store, p := resourceFixtureThrough(t, "00009")
		if i == 0 {
			if _, err = p.Exec(ctx, `INSERT INTO requests(id,type,location,details,status,assigned_resource_id) VALUES($1,'WATER',ST_MakePoint(2,2),'old request','COMPLETED',$2)`, request, legacy); err != nil {
				t.Fatal(err)
			}
			if _, err = p.Exec(ctx, `INSERT INTO assignment_attempts(assignment_id,request_id,resource_id,generation,distance_meters,payload_hash,phase) VALUES($1,$1,$2,1,0,'fixture','FINALIZED')`, request, legacy); err != nil {
				t.Fatal(err)
			}
			if _, err = p.Exec(ctx, `INSERT INTO notifications(id,request_id,assignment_id,resource_id,recipient,created_at) VALUES($1,$1,$1,$2,'demo','2026-11-05 15:30 UTC')`, request, legacy); err != nil {
				t.Fatal(err)
			}
		} else {
			id := legacy
			if i == 1 {
				id = undated
			}
			if _, err = p.Exec(ctx, `INSERT INTO resources(id,type,location,availability,status,assigned_request_id) VALUES($1,'WATER',ST_MakePoint(3,4),false,'ALLOCATED',$2)`, id, request); err != nil {
				t.Fatal(err)
			}
		}
		if i == 2 {
			// More than fifty undated resource rows hide the dated legacy owner
			// from its resource-shard window. Hydration must still recover it.
			if _, err = p.Exec(ctx, `INSERT INTO resources(id,type,location,availability,status,assigned_request_id) SELECT repeat('f',24)||lpad(to_hex(i),8,'0'),'WATER',ST_MakePoint(0,0),false,'ALLOCATED',$1 FROM generate_series(1,60) i`, request); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = p.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(httpapp.NewServer(store, secret, true))
		t.Cleanup(server.Close)
		urls[i] = server.URL
	}
	r := coordinator.NewRouter(urls, secret)
	coord := httptest.NewServer(httpapp.NewServer(r, secret, true))
	defer coord.Close()
	api := httptest.NewServer(httpapp.NewServer(httpapp.NewClient(coord.URL, secret), "", false))
	defer api.Close()
	client := httpapp.NewPublicClient(api.URL)
	page, err := client.ListResources(ctx, contracts.ListQuery{Sort: "allocated", Status: "ALLOCATED", Type: "WATER", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 50 || page.Items[0].ID != legacy || !page.Items[1].AllocationOrderTime().IsZero() {
		t.Fatal(page)
	}
	legacyResource, err := client.GetResource(ctx, legacy)
	if err != nil || !legacyResource.AllocatedAt.IsZero() || !legacyResource.AllocationConfirmedAt.Equal(time.Date(2026, 11, 5, 15, 30, 0, 0, time.UTC)) || !page.Items[0].AllocationConfirmedAt.Equal(legacyResource.AllocationConfirmedAt) {
		t.Fatal(legacyResource, err)
	}
	unknown, err := client.GetResource(ctx, undated)
	if err != nil || !unknown.AllocationOrderTime().IsZero() {
		t.Fatal(unknown, err)
	}
	if page.Items[0].Location == nil || *page.Items[0].Location.Latitude != 4 || page.Items[0].AssignedRequestID != request {
		t.Fatal(page.Items[0])
	}
	urls[0] = ""
	partial, err := coordinator.NewRouter(urls, secret).GetResource(ctx, legacy)
	if err != nil || partial.ID != legacy || !partial.AllocationConfirmationUnavailable {
		t.Fatalf("resource details during confirmation shard outage: %+v, %v", partial, err)
	}
	if unknown.AllocationConfirmationUnavailable || legacyResource.AllocationConfirmationUnavailable {
		t.Fatal("successful confirmation checks must not require a retry")
	}
}
