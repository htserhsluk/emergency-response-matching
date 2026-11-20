package coordinator

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"sort"
)

func (r *Router) MapLocations(ctx context.Context, kind, after string) (contracts.MapPage, error) {
	if !contracts.ValidMapQuery(kind, after) {
		return contracts.MapPage{}, contracts.ErrInvalid
	}
	items, err := fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.MapLocation, error) {
		p, e := n.MapLocations(ctx, kind, after)
		return p.Items, e
	})
	if err != nil {
		return contracts.MapPage{}, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Cursor() < items[j].Cursor() })
	page := contracts.MapPage{Items: items}
	if len(items) > contracts.MapPageSize {
		page.Items = items[:contracts.MapPageSize]
		page.Next = page.Items[len(page.Items)-1].Cursor()
	}
	return page, nil
}

func (r *Router) RequestCandidates(ctx context.Context, id string) (contracts.RequestPreview, error) {
	out := contracts.RequestPreview{Items: []contracts.NearbyRequest{}}
	resource, err := r.GetResource(ctx, id)
	if err != nil {
		return out, err
	}
	if resource.AssignedRequestID != "" {
		n, e := r.node(resource.AssignedRequestID)
		if e != nil {
			return out, e
		}
		req, e := n.Get(ctx, resource.AssignedRequestID)
		if e != nil {
			return out, e
		}
		if req.Assignment != nil && req.AssignedResourceID == id && (req.Status == "ASSIGNED" || req.Status == "COMPLETED") {
			out.AssignedRequest = &contracts.NearbyRequest{ID: req.ID, Location: req.Location, DistanceMeters: req.Assignment.DistanceMeters}
		}
		return out, nil
	}
	q := contracts.NearbyQuery{Type: resource.Type, Location: resource.Location}
	items, e := fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.NearbyRequest, error) {
		return n.NearbyRequests(ctx, q)
	})
	if e != nil {
		return out, e
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DistanceMeters == items[j].DistanceMeters {
			return items[i].ID < items[j].ID
		}
		return items[i].DistanceMeters < items[j].DistanceMeters
	})
	if len(items) > 5 {
		items = items[:5]
	}
	out.Items = items
	return out, nil
}
