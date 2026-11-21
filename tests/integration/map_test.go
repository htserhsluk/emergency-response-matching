//go:build integration

package integration

import (
	"context"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"net/http/httptest"
	"testing"
)

func TestMapLocationsAndFiveNearestRequests(t *testing.T) {
	ctx := context.Background()
	const secret = "map-preview-integration-secret"
	var urls [3]string
	id := contracts.EntityID("resource", "map-preview")
	owner, _ := routing.ShardFor(id)
	for shard := range 3 {
		s, pool := resourceFixture(t)
		server := httptest.NewServer(httpapp.NewServer(s, secret, true))
		t.Cleanup(server.Close)
		urls[shard] = server.URL
		_, err := pool.Exec(ctx, `INSERT INTO requests(id,type,location,details,status)
			SELECT md5($1::text || '/' || i), 'MEDICINE', ST_SetSRID(ST_MakePoint(0,0),4326)::geography, 'pagination','PENDING'
			FROM generate_series(1,40) AS i`, fmt.Sprint(shard))
		if err != nil {
			t.Fatal(err)
		}
		for j, lon := range []float64{float64(shard + 1), float64(shard + 4), 100} {
			key := fmt.Sprintf("%032x", 1+shard+3*j)
			_, err := pool.Exec(ctx, `INSERT INTO requests(id,type,location,details,status) VALUES($1,'WATER',ST_SetSRID(ST_MakePoint($2,0),4326)::geography,'preview','UNMATCHED')`, key, lon)
			if err != nil {
				t.Fatal(err)
			}
		}
		if int(owner) == shard {
			_, err := pool.Exec(ctx, `INSERT INTO resources(id,type,location,availability,status) VALUES($1,'WATER',ST_SetSRID(ST_MakePoint(0,0),4326)::geography,false,'UNAVAILABLE')`, id)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	router := coordinator.NewRouter(urls, secret)
	api := httptest.NewServer(httpapp.NewServer(router, "", false))
	defer api.Close()
	client := httpapp.NewPublicClient(api.URL)
	counts := map[string]int{}
	after := ""
	for pageNumber := 0; ; pageNumber++ {
		page, err := client.MapLocations(ctx, "ALL", after)
		if err != nil || pageNumber > 1 {
			t.Fatalf("all locations page %d: %v", pageNumber, err)
		}
		for _, record := range page.Items {
			counts[record.Kind+"/"+record.Type]++
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	if len(counts) != 3 || counts["request/MEDICINE"] != 120 || counts["request/WATER"] != 9 || counts["resource/WATER"] != 1 {
		t.Fatalf("all locations lost records or types: %v", counts)
	}
	first, err := client.MapLocations(ctx, "MEDICINE", "")
	if err != nil || len(first.Items) != 100 || first.Next == "" {
		t.Fatalf("first map page: %d, %v", len(first.Items), err)
	}
	last, err := client.MapLocations(ctx, "MEDICINE", first.Next)
	if err != nil || len(last.Items) != 20 || last.Next != "" || last.Items[0].Cursor() <= first.Next {
		t.Fatalf("last map page: %d, %v", len(last.Items), err)
	}
	page, err := client.MapLocations(ctx, "WATER", "")
	if err != nil || len(page.Items) != 10 || page.Next != "" {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
	other, err := client.MapLocations(ctx, "FOOD", "")
	if err != nil || len(other.Items) != 0 {
		t.Fatalf("food page = %+v, err = %v", other, err)
	}
	preview, err := client.RequestCandidates(ctx, id)
	if err != nil || len(preview.Items) != 5 || preview.AssignedRequest != nil {
		t.Fatalf("preview = %+v, err = %v", preview, err)
	}
	for i, item := range preview.Items {
		if item.ID != fmt.Sprintf("%032x", i+1) || item.DistanceMeters <= 0 {
			t.Fatalf("unexpected candidate %d = %+v", i, item)
		}
	}
}
