package coordinator

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
)

func (r *Router) ListNotifications(ctx context.Context, id string, q contracts.ListQuery) (contracts.NotificationPage, error) {
	n, err := r.node(id)
	if err != nil {
		return contracts.NotificationPage{}, err
	}
	return n.ListNotifications(ctx, id, q)
}
func (r *Router) ReadNotification(ctx context.Context, requestID, id, key string) (contracts.Notification, error) {
	unlock, err := r.lockRequest(ctx, requestID)
	if err != nil {
		return contracts.Notification{}, err
	}
	defer unlock()
	n, err := r.node(requestID)
	if err != nil {
		return contracts.Notification{}, err
	}
	return n.ReadNotification(ctx, requestID, id, key)
}
