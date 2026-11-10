//go:build integration

package integration

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTypedInternalCRUDProtectsHistory(t *testing.T) {
	s, p := resourceFixture(t)
	secret := "internal-fixture-secret-32-characters"
	server := httptest.NewServer(httpapp.NewServer(s, secret, true))
	defer server.Close()
	key := "internal-create-00001"
	id := contracts.EntityID("register-resource", key)
	call := func(method, path, body, credential string, want int) {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("X-Internal-Secret", credential)
		r.Header.Set("Idempotency-Key", key)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s %s status=%d want=%d", method, path, resp.StatusCode, want)
		}
	}
	payload := `{"operation":"register-resource","resource":{"type":"WATER","location":{"latitude":0,"longitude":0},"availability":true}}`
	call("PUT", "/internal/data/resource:"+id, payload, "", 401)
	call("PUT", "/internal/data/resource:"+id, payload, secret, 200)
	call("GET", "/internal/data/resource:"+id, "", secret, 200)
	call("DELETE", "/internal/data/resource:"+id, "", secret, 409)
	call("PUT", "/internal/data/resource:00000000000000000000000000000000", payload, secret, 400)
	fixtureID := "00000000000000000000000000000009"
	_, err := p.Exec(context.Background(), `INSERT INTO resources(id,type,location,availability,status) VALUES($1,'WATER',ST_SetSRID(ST_MakePoint(0,0),4326)::geography,false,'UNAVAILABLE')`, fixtureID)
	if err != nil {
		t.Fatal(err)
	}
	call("DELETE", "/internal/data/resource:"+fixtureID, "", secret, 200)
	call("GET", "/internal/data/resource:"+fixtureID, "", secret, 404)
}
