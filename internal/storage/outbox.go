package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"time"
)

func (s *Store) ClaimOutbox(ctx context.Context, token string) ([]contracts.Delivery, error) {
	if contracts.ValidateKey(token) != nil {
		return nil, contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup(tx)
	rows, err := tx.Query(ctx, `WITH due AS (SELECT delivery_id FROM outbox_events WHERE sent_at IS NULL AND due_at<=clock_timestamp() AND (claim_until IS NULL OR claim_until<clock_timestamp()) ORDER BY due_at,delivery_id LIMIT 1 FOR UPDATE SKIP LOCKED) UPDATE outbox_events o SET claim_token=$1,claim_until=clock_timestamp()+interval '30 seconds',durability_counter=durability_counter+1 FROM due WHERE o.delivery_id=due.delivery_id RETURNING o.delivery_id,o.payload,o.retry_number,o.destination,o.claim_token`, token)
	if err != nil {
		return nil, err
	}
	out := []contracts.Delivery{}
	for rows.Next() {
		var d contracts.Delivery
		var data []byte
		if err = rows.Scan(&d.DeliveryID, &data, &d.RetryNumber, &d.Destination, &d.ClaimToken); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(data, &d.Event); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = commit(ctx, tx); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Store) MarkSent(ctx context.Context, request, id, token string) error {
	if contracts.ValidateKey(token) != nil {
		return contracts.ErrInvalid
	}
	if _, err := routing.ShardFor(request); err != nil {
		return contracts.ErrInvalid
	}
	if _, err := routing.ShardFor(id); err != nil {
		return contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer cleanup(tx)
	tag, err := tx.Exec(ctx, `UPDATE outbox_events SET sent_at=coalesce(sent_at,clock_timestamp()),durability_counter=durability_counter+1 WHERE delivery_id=$1 AND request_id=$2 AND claim_token=$3`, id, request, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return contracts.ErrConflict
	}
	return commit(ctx, tx)
}
func (s *Store) ScheduleMatch(ctx context.Context, id, key string) (contracts.Request, error) {
	var out contracts.Request
	if contracts.ValidateKey(key) != nil {
		return out, contracts.ErrInvalid
	}
	if _, err := routing.ShardFor(id); err != nil {
		return out, contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id); err != nil {
		return out, err
	}
	hash := contracts.PayloadHash(struct{}{})
	found, err := replay(ctx, tx, id, "schedule-match", key, hash, &out)
	if errors.Is(err, contracts.ErrConflict) {
		return out, confirmConflict(ctx, tx)
	}
	if err != nil {
		return out, err
	}
	if found {
		return out, commit(ctx, tx)
	}
	out, err = scan(tx.QueryRow(ctx, selectRequest+" FOR UPDATE", id))
	if err != nil {
		return out, err
	}
	if out.Status == "UNMATCHED" {
		out.Status = "PENDING"
		out.Generation++
		if _, err = tx.Exec(ctx, `UPDATE requests SET status='PENDING',generation=$2,version=version+1 WHERE id=$1`, id, out.Generation); err != nil {
			return out, err
		}
		event := contracts.MatchEvent{SchemaVersion: 1, EventID: contracts.EntityID("match-event", fmt.Sprintf("%s:%d", id, out.Generation)), RequestID: id, Generation: out.Generation, CreatedAt: time.Now().UTC()}
		data, _ := json.Marshal(event)
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(delivery_id,event_id,request_id,generation,payload) VALUES($1,$2,$3,$4,$5)`, contracts.DeliveryID(event.EventID, 0, "request.match"), event.EventID, id, out.Generation, data); err != nil {
			return out, err
		}
	}
	if err = record(ctx, tx, id, "schedule-match", key, hash, out); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}
