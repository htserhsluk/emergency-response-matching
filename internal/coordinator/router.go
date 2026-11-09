package coordinator

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
)

type Router struct {
	nodes    [3]*httpapp.Client
	Registry *Registry
	NodeIDs  [3]string
}

func NewRouter(urls [3]string, secret string) *Router {
	r := &Router{}
	for i, url := range urls {
		if url != "" {
			r.nodes[i] = httpapp.NewClient(url, secret)
		}
	}
	return r
}
func (r *Router) node(id string) (*httpapp.Client, error) {
	shard, err := routing.ShardFor(id)
	if err != nil {
		return nil, contracts.ErrInvalid
	}
	if r.nodes[shard] == nil {
		return nil, contracts.ErrUnavailable
	}
	if r.Registry != nil && !r.Registry.Writable(r.NodeIDs[shard]) {
		return nil, contracts.ErrUnavailable
	}
	return r.nodes[shard], nil
}
func (r *Router) Create(ctx context.Context, key string, c contracts.CreateRequest) (contracts.Request, error) {
	n, err := r.node(contracts.EntityID("create-request", key))
	if err != nil {
		return contracts.Request{}, err
	}
	return n.Create(ctx, key, c)
}
func (r *Router) Get(ctx context.Context, id string) (contracts.Request, error) {
	n, err := r.node(id)
	if err != nil {
		return contracts.Request{}, err
	}
	return n.Get(ctx, id)
}
func (r *Router) SetStatus(ctx context.Context, id, key, status string) (contracts.Request, error) {
	n, err := r.node(id)
	if err != nil {
		return contracts.Request{}, err
	}
	return n.SetStatus(ctx, id, key, status)
}
func (r *Router) Health(ctx context.Context) error {
	count := 0
	for i, n := range r.nodes {
		if n != nil {
			if r.Registry != nil && !r.Registry.Writable(r.NodeIDs[i]) {
				return contracts.ErrUnavailable
			}
			count++
			if err := n.Health(ctx); err != nil {
				return err
			}
		}
	}
	if count == 0 {
		return contracts.ErrUnavailable
	}
	return nil
}
