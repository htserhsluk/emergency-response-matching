package coordinator

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
)

func (r *Router) ScheduleMatch(ctx context.Context, id, key string) (contracts.Request, error) {
	n, err := r.node(id)
	if err != nil {
		return contracts.Request{}, err
	}
	return n.ScheduleMatch(ctx, id, key)
}
func (r *Router) BeginIntent(ctx context.Context, c contracts.IntentCommand) (contracts.Intent, error) {
	if c.Validate() != nil {
		return contracts.Intent{}, contracts.ErrInvalid
	}
	n, err := r.node(c.RequestID)
	if err != nil {
		return contracts.Intent{}, err
	}
	return n.BeginIntent(ctx, c)
}
func (r *Router) Reserve(ctx context.Context, c contracts.ReserveCommand) (contracts.ReservationResult, error) {
	if c.Validate() != nil {
		return contracts.ReservationResult{}, contracts.ErrInvalid
	}
	n, err := r.node(c.ResourceID)
	if err != nil {
		return contracts.ReservationResult{}, err
	}
	return n.Reserve(ctx, c)
}
func (r *Router) ClaimOutbox(ctx context.Context, token string) ([]contracts.Delivery, error) {
	if contracts.ValidateKey(token) != nil {
		return nil, contracts.ErrInvalid
	}
	return fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.Delivery, error) {
		return n.ClaimOutbox(ctx, token)
	})
}
func (r *Router) MarkSent(ctx context.Context, request, id, token string) error {
	n, err := r.node(request)
	if err != nil {
		return err
	}
	return n.MarkSent(ctx, request, id, token)
}
