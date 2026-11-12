package storage

import (
	"context"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"github.com/jackc/pgx/v5"
)

const notificationColumns = `id,request_id,assignment_id,resource_id,recipient,created_at,read_at`

func scanNotification(row pgx.Row) (contracts.Notification, error) {
	var n contracts.Notification
	err := row.Scan(&n.ID, &n.RequestID, &n.AssignmentID, &n.ResourceID, &n.Recipient, &n.CreatedAt, &n.ReadAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = contracts.ErrNotFound
	}
	return n, err
}
func (s *Store) ListNotifications(ctx context.Context, id string, q contracts.ListQuery) (contracts.NotificationPage, error) {
	out := contracts.NotificationPage{Items: []contracts.Notification{}}
	if _, err := routing.ShardFor(id); err != nil || q.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM requests WHERE id=$1)`, id).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, contracts.ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT `+notificationColumns+` FROM notifications WHERE request_id=$1 AND id>$2 ORDER BY id LIMIT $3`, id, q.After, q.Limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, n)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		out.NextAfter = out.Items[len(out.Items)-1].ID
	}
	return out, nil
}
func (s *Store) ReadNotification(ctx context.Context, requestID, id, key string) (contracts.Notification, error) {
	var out contracts.Notification
	if _, err := routing.ShardFor(requestID); err != nil {
		return out, contracts.ErrInvalid
	}
	if _, err := routing.ShardFor(id); err != nil || contracts.ValidateKey(key) != nil {
		return out, contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, requestID); err != nil {
		return out, err
	}
	// Check parent ownership before replay, including a caller-supplied key.
	out, err = scanNotification(tx.QueryRow(ctx, `SELECT `+notificationColumns+` FROM notifications WHERE id=$1 AND request_id=$2 FOR UPDATE`, id, requestID))
	if err != nil {
		return out, err
	}
	kind := "read-notification:" + id
	hash := contracts.PayloadHash(struct{ Read bool }{true})
	found, err := replay(ctx, tx, requestID, kind, key, hash, &out)
	if err != nil {
		return out, err
	}
	if found {
		return out, commit(ctx, tx)
	}
	out, err = scanNotification(tx.QueryRow(ctx, `UPDATE notifications SET read_at=coalesce(read_at,clock_timestamp()),durability_counter=durability_counter+1 WHERE id=$1 AND request_id=$2 RETURNING `+notificationColumns, id, requestID))
	if err != nil {
		return out, err
	}
	if err = record(ctx, tx, requestID, kind, key, hash, out); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}
