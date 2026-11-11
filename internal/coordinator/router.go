package coordinator

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"sync"
)

type Router struct {
	lockInit sync.Once
	locks    [256]chan struct{}
	Control  *Control
	Topology map[string]*httpapp.Client
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
	return r.shardNode(shard)
}
func (r *Router) shardNode(shard int) (*httpapp.Client, error) {
	if r.Control != nil {
		route := r.Control.Route(shard)
		n := r.Topology[route.NodeID]
		if n == nil || r.Registry == nil || !r.Registry.Writable(route.NodeID) {
			return nil, contracts.ErrUnavailable
		}
		return n.WithVersion(route.Version), nil
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
	unlock, err := r.lockRequest(ctx, id)
	if err != nil {
		return contracts.Request{}, err
	}
	defer unlock()
	n, err := r.node(id)
	if err != nil {
		return contracts.Request{}, err
	}
	return n.SetStatus(ctx, id, key, status)
}
func (r *Router) Health(ctx context.Context) error {
	if r.Control != nil {
		for i := range 3 {
			n, err := r.shardNode(i)
			if err != nil {
				return err
			}
			if err = n.Health(ctx); err != nil {
				return err
			}
		}
		return nil
	}
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
