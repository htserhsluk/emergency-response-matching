package httpapp

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"net/http"
	"net/url"
)

type mapReader interface {
	MapLocations(context.Context, string, string) (contracts.MapPage, error)
}
type nearbyReader interface {
	NearbyRequests(context.Context, contracts.NearbyQuery) ([]contracts.NearbyRequest, error)
}
type candidateReader interface {
	RequestCandidates(context.Context, string) (contracts.RequestPreview, error)
}

func serveMap(w http.ResponseWriter, r *http.Request, store Requests, parts []string, internal bool) {
	var result any
	var err error
	switch {
	case len(parts) == 2 && parts[0] == "map" && parts[1] == "locations" && r.Method == "GET":
		kind, after := r.URL.Query().Get("type"), r.URL.Query().Get("after")
		if !contracts.ValidMapQuery(kind, after) {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		s, ok := store.(mapReader)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		result, err = s.MapLocations(r.Context(), kind, after)
	case internal && len(parts) == 2 && parts[0] == "map" && parts[1] == "nearby" && r.Method == "POST":
		var q contracts.NearbyQuery
		if !decode(w, r, &q) {
			return
		}
		if q.Validate() != nil {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		s, ok := store.(nearbyReader)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		result, err = s.NearbyRequests(r.Context(), q)
	case len(parts) == 3 && parts[0] == "resources" && parts[2] == "candidates" && r.Method == "GET":
		if _, e := routing.ShardFor(parts[1]); e != nil {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		s, ok := store.(candidateReader)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		result, err = s.RequestCandidates(r.Context(), parts[1])
	default:
		fail(w, contracts.ErrNotFound, "")
		return
	}
	if err != nil {
		fail(w, err, "")
		return
	}
	write(w, 200, result)
}

func (c *Client) MapLocations(ctx context.Context, kind, after string) (out contracts.MapPage, err error) {
	err = c.exchange(ctx, "GET", "/map/locations?type="+url.QueryEscape(kind)+"&after="+url.QueryEscape(after), "", nil, &out)
	return
}
func (c *Client) NearbyRequests(ctx context.Context, q contracts.NearbyQuery) (out []contracts.NearbyRequest, err error) {
	err = c.exchange(ctx, "POST", "/map/nearby", "", q, &out)
	return
}
func (c *Client) RequestCandidates(ctx context.Context, id string) (out contracts.RequestPreview, err error) {
	err = c.exchange(ctx, "GET", "/resources/"+id+"/candidates", "", nil, &out)
	return
}
