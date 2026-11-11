package storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"github.com/jackc/pgx/v5"
	"time"
)

// Candidate identity/type/location are immutable. Availability is deliberately
// left to Reserve's atomic conditional update, including during reconciliation.
func (s *Store) CheckCandidate(ctx context.Context, q contracts.CandidateQuery) (contracts.Candidate, error) {
	var out contracts.Candidate
	if _, err := routing.ShardFor(q.ResourceID); err != nil {
		return out, contracts.ErrInvalid
	}
	if (contracts.CreateRequest{Type: q.Type, Location: q.Location, Details: "candidate"}).Validate() != nil {
		return out, contracts.ErrInvalid
	}
	err := s.pool.QueryRow(ctx, `SELECT id,ST_Distance(location,ST_SetSRID(ST_MakePoint($2,$3),4326)::geography) FROM resources WHERE id=$1 AND type=$4`, q.ResourceID, *q.Location.Longitude, *q.Location.Latitude, q.Type).Scan(&out.ResourceID, &out.DistanceMeters)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, contracts.ErrInvalid
	}
	return out, err
}

func (s *Store) IntentForReservation(ctx context.Context, c contracts.ReserveCommand) (contracts.Intent, error) {
	var out contracts.Intent
	if c.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.RequestID); err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `UPDATE assignment_attempts a SET durability_counter=a.durability_counter+1 FROM requests r WHERE a.assignment_id=$1 AND a.request_id=$2 AND a.resource_id=$3 AND r.id=a.request_id AND r.generation=a.generation AND a.phase IN ('INTENT','RESERVED','FINALIZED') AND NOT EXISTS(SELECT 1 FROM processed_events p WHERE p.request_id=r.id AND p.generation=r.generation AND p.state='PAUSED') RETURNING a.assignment_id,a.request_id,a.resource_id,a.generation,a.distance_meters,a.phase`, c.AssignmentID, c.RequestID, c.ResourceID).Scan(&out.AssignmentID, &out.RequestID, &out.ResourceID, &out.Generation, &out.DistanceMeters, &out.Phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, contracts.ErrConflict
	}
	if err != nil {
		return out, err
	}
	if err = commit(ctx, tx); err != nil {
		return contracts.Intent{}, err
	}
	return out, nil
}

// Workflow confirms every decision with a WAL-generating transaction. Its result
// must never authorize an acknowledgement or another shard call when err != nil.
func (s *Store) Workflow(ctx context.Context, c contracts.WorkflowCommand) (contracts.WorkflowState, error) {
	var out contracts.WorkflowState
	if c.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	d := c.Delivery
	e := d.Event
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, e.RequestID); err != nil {
		return out, err
	}
	out.Request, err = scan(tx.QueryRow(ctx, selectRequest+" FOR UPDATE", e.RequestID))
	if err != nil {
		return out, err
	}
	// The original durable event is authoritative, including its schema/timestamp.
	var payload []byte
	err = tx.QueryRow(ctx, `SELECT payload FROM outbox_events WHERE request_id=$1 AND generation=$2 AND event_id=$3 AND retry_number=0 AND destination='request.match'`, e.RequestID, e.Generation, e.EventID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, contracts.ErrInvalid
	}
	if err != nil {
		return out, err
	}
	var original contracts.MatchEvent
	if err = json.Unmarshal(payload, &original); err != nil {
		return out, err
	}
	if original.SchemaVersion != e.SchemaVersion || original.RequestID != e.RequestID || original.EventID != e.EventID || original.Generation != e.Generation || !original.CreatedAt.Equal(e.CreatedAt) {
		return out, contracts.ErrInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE requests SET version=version+1 WHERE id=$1`, e.RequestID); err != nil {
		return out, err
	}
	finish := func() (contracts.WorkflowState, error) {
		if err := commit(ctx, tx); err != nil {
			return contracts.WorkflowState{}, err
		}
		return out, nil
	}
	if e.Generation < out.Request.Generation {
		out.Ack = true
		out.State = "DONE"
		return finish()
	}
	if e.Generation != out.Request.Generation {
		return out, contracts.ErrInvalid
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processed_events(event_id,request_id,generation,state) VALUES($1,$2,$3,'RUNNING') ON CONFLICT DO NOTHING`, e.EventID, e.RequestID, e.Generation); err != nil {
		return out, err
	}
	var limit int
	var databaseNow time.Time
	err = tx.QueryRow(ctx, `UPDATE processed_events SET durability_counter=durability_counter+1 WHERE event_id=$1 RETURNING state,expected_retry,retry_limit,due_at,clock_timestamp()`, e.EventID).Scan(&out.State, &out.ExpectedRetry, &limit, &out.DueAt, &databaseNow)
	if err != nil {
		return out, err
	}
	if d.RetryNumber > out.ExpectedRetry {
		return out, contracts.ErrInvalid
	}
	// A redrive is conditional on the paused attempt: repeating it cannot add work.
	if c.Action == "redrive" && out.State == "PAUSED" && d.RetryNumber == out.ExpectedRetry {
		out.ExpectedRetry++
		out.State = "RUNNING"
		out.DueAt = databaseNow
		if _, err = tx.Exec(ctx, `UPDATE processed_events SET state='RUNNING',expected_retry=$2,retry_limit=$2+3,due_at=$3,last_error='' WHERE event_id=$1`, e.EventID, out.ExpectedRetry, out.DueAt); err != nil {
			return out, err
		}
		if err = insertDelivery(ctx, tx, e, out.ExpectedRetry, "request.match", out.DueAt); err != nil {
			return out, err
		}
		return finish()
	}
	if d.RetryNumber < out.ExpectedRetry || out.State == "DONE" || out.State == "PAUSED" {
		out.Ack = true
		return finish()
	}
	if c.Action == "redrive" {
		return finish()
	}
	if out.DueAt.After(databaseNow) {
		return out, contracts.ErrUnavailable
	}
	var intent contracts.Intent
	err = tx.QueryRow(ctx, `UPDATE assignment_attempts SET durability_counter=durability_counter+1 WHERE request_id=$1 AND generation=$2 AND phase IN ('INTENT','RESERVED','FINALIZED') RETURNING assignment_id,request_id,resource_id,generation,distance_meters,phase`, e.RequestID, e.Generation).Scan(&intent.AssignmentID, &intent.RequestID, &intent.ResourceID, &intent.Generation, &intent.DistanceMeters, &intent.Phase)
	if err == nil {
		out.Intent = &intent
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if out.Request.Status == "ASSIGNED" || out.Request.Status == "COMPLETED" || out.Request.Status == "UNMATCHED" {
		out.State = "DONE"
		out.Ack = true
		if _, err = tx.Exec(ctx, `UPDATE processed_events SET state='DONE' WHERE event_id=$1`, e.EventID); err != nil {
			return out, err
		}
		return finish()
	}
	switch c.Action {
	case "inspect":
	case "finalize", "reject":
		if out.Intent == nil || out.Intent.AssignmentID != c.AssignmentID || out.Intent.Phase != "INTENT" || out.Request.Status != "ASSIGNING" {
			return out, contracts.ErrConflict
		}
		phase, status := "FINALIZED", "ASSIGNED"
		var resource any = intent.ResourceID
		if c.Action == "reject" {
			phase, status, resource = "REJECTED", "PENDING", nil
		}
		if _, err = tx.Exec(ctx, `UPDATE assignment_attempts SET phase=$2 WHERE assignment_id=$1`, intent.AssignmentID, phase); err != nil {
			return out, err
		}
		if _, err = tx.Exec(ctx, `UPDATE requests SET status=$2,assigned_resource_id=$3,version=version+1 WHERE id=$1`, e.RequestID, status, resource); err != nil {
			return out, err
		}
		out.Request.Status = status
		out.Intent.Phase = phase
		if c.Action == "finalize" {
			out.Request.AssignedResourceID = intent.ResourceID
			out.State = "DONE"
			out.Ack = true
			if _, err = tx.Exec(ctx, `UPDATE processed_events SET state='DONE' WHERE event_id=$1`, e.EventID); err != nil {
				return out, err
			}
		} else {
			out.Intent = nil
		}
	case "unmatched":
		if out.Intent != nil || out.Request.Status != "PENDING" {
			return out, contracts.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE requests SET status='UNMATCHED',version=version+1 WHERE id=$1`, e.RequestID); err != nil {
			return out, err
		}
		if _, err = tx.Exec(ctx, `UPDATE processed_events SET state='DONE' WHERE event_id=$1`, e.EventID); err != nil {
			return out, err
		}
		out.Request.Status = "UNMATCHED"
		out.State = "DONE"
		out.Ack = true
	case "retry", "pause":
		destination := "request.match"
		if c.Action == "pause" || out.ExpectedRetry >= limit {
			out.State = "PAUSED"
			destination = "request.dead"
		} else {
			delays := []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
			used := out.ExpectedRetry - (limit - 3)
			if used < 0 || used > 2 {
				return out, contracts.ErrConflict
			}
			out.DueAt = databaseNow.Add(delays[used])
			out.ExpectedRetry++
		}
		if _, err = tx.Exec(ctx, `UPDATE processed_events SET state=$2,expected_retry=$3,due_at=$4,last_error=$5 WHERE event_id=$1`, e.EventID, out.State, out.ExpectedRetry, out.DueAt, c.Reason); err != nil {
			return out, err
		}
		if err = insertDelivery(ctx, tx, e, out.ExpectedRetry, destination, out.DueAt); err != nil {
			return out, err
		}
		out.Ack = true
	}
	return finish()
}

func insertDelivery(ctx context.Context, tx pgx.Tx, e contracts.MatchEvent, retry int, destination string, due time.Time) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(delivery_id,event_id,request_id,generation,payload,retry_number,destination,due_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(delivery_id) DO NOTHING`, contracts.DeliveryID(e.EventID, retry, destination), e.EventID, e.RequestID, e.Generation, payload, retry, destination, due)
	return err
}

// Scanning does not authorize side effects; Workflow(inspect) supplies the barrier.
func (s *Store) PendingWork(ctx context.Context) ([]contracts.Delivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT o.payload,o.delivery_id,o.retry_number,o.destination FROM processed_events p JOIN outbox_events o ON o.event_id=p.event_id AND o.retry_number=p.expected_retry AND o.destination='request.match' WHERE p.state='RUNNING' AND p.due_at<=clock_timestamp() ORDER BY p.due_at,p.event_id LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Delivery{}
	for rows.Next() {
		var d contracts.Delivery
		var payload []byte
		if err = rows.Scan(&payload, &d.DeliveryID, &d.RetryNumber, &d.Destination); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(payload, &d.Event); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
