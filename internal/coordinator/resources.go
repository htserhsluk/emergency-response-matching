package coordinator

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"sort"
)

func (r *Router) DeleteData(ctx context.Context, kind, id string) error {
	if kind != "request" && kind != "resource" {
		return contracts.ErrInvalid
	}
	n, err := r.node(id)
	if err != nil {
		return err
	}
	return n.DeleteData(ctx, kind, id)
}

func (r *Router) RegisterResource(ctx context.Context, key string, c contracts.RegisterResource) (contracts.Resource, error) {
	if contracts.ValidateKey(key) != nil || c.Validate() != nil {
		return contracts.Resource{}, contracts.ErrInvalid
	}
	n, err := r.node(contracts.EntityID("register-resource", key))
	if err != nil {
		return contracts.Resource{}, err
	}
	return n.RegisterResource(ctx, key, c)
}
func (r *Router) GetResource(ctx context.Context, id string) (contracts.Resource, error) {
	n, err := r.node(id)
	if err != nil {
		return contracts.Resource{}, err
	}
	return n.GetResource(ctx, id)
}
func (r *Router) SetResourceStatus(ctx context.Context, id, key, status string) (contracts.Resource, error) {
	n, err := r.node(id)
	if err != nil {
		return contracts.Resource{}, err
	}
	return n.SetResourceStatus(ctx, id, key, status)
}

// Every shard must respond. A partial result cannot prove global availability or nearest distance.
func fanout[T any](ctx context.Context, r *Router, fetch func(context.Context, *httpapp.Client) ([]T, error)) ([]T, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		items []T
		err   error
	}
	ch := make(chan result, 3)
	for i := range 3 {
		n, err := r.shardNode(i)
		if err != nil {
			return nil, err
		}
		go func() { items, err := fetch(ctx, n); ch <- result{items, err} }()
	}
	out := []T{}
	for range 3 {
		got := <-ch
		if got.err != nil {
			return nil, contracts.ErrUnavailable
		}
		out = append(out, got.items...)
	}
	return out, nil
}
func (r *Router) ListResources(ctx context.Context, q contracts.ListQuery) (contracts.ResourcePage, error) {
	if q.Validate() != nil {
		return contracts.ResourcePage{}, contracts.ErrInvalid
	}
	items, err := fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.Resource, error) {
		p, e := n.ListResources(ctx, q)
		return p.Items, e
	})
	if err != nil {
		return contracts.ResourcePage{}, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if len(items) > q.Limit {
		items = items[:q.Limit]
	}
	out := contracts.ResourcePage{Items: items}
	if len(items) > 0 {
		out.Next = items[len(items)-1].ID
	}
	return out, nil
}
func (r *Router) Search(ctx context.Context, q contracts.SearchQuery) ([]contracts.Candidate, error) {
	if q.Validate() != nil {
		return nil, contracts.ErrInvalid
	}
	items, err := fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.Candidate, error) { return n.Search(ctx, q) })
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DistanceMeters == items[j].DistanceMeters {
			return items[i].ResourceID < items[j].ResourceID
		}
		return items[i].DistanceMeters < items[j].DistanceMeters
	})
	if len(items) > q.Limit {
		items = items[:q.Limit]
	}
	return items, nil
}
