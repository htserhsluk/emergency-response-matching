package coordinator

import (
	"context"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"golang.org/x/sync/errgroup"
)

func (r *Router) allocationHistory(ctx context.Context, q contracts.ListQuery) (contracts.ResourcePage, error) {
	window := q
	window.After = ""
	window.Limit = contracts.AllocationHistoryLimit
	rows, err := fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.Resource, error) {
		p, err := n.ListResources(ctx, window)
		return p.Items, err
	})
	if err != nil {
		return contracts.ResourcePage{}, err
	}
	page := contracts.AllocationPage(rows, q)
	// Legacy timestamps live on request shards. Hydrate only displayed rows that
	// were absent from their resource shard's bounded recent window.
	group, lookup := errgroup.WithContext(ctx)
	group.SetLimit(8)
	for i, summary := range page.Items {
		if summary.Location != nil {
			continue
		}
		group.Go(func() error {
			n, err := r.node(summary.ID)
			if err != nil {
				return err
			}
			resource, err := n.GetResource(lookup, summary.ID)
			if err != nil {
				return err
			}
			if resource.Status != "ALLOCATED" || resource.AssignedRequestID != summary.AssignedRequestID {
				return contracts.ErrUnavailable
			}
			resource.AllocationConfirmedAt = summary.AllocationConfirmedAt
			page.Items[i] = resource
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return contracts.ResourcePage{}, err
	}
	return page, nil
}
