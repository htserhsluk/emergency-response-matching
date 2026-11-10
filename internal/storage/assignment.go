package storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Reserve(ctx context.Context, c contracts.ReserveCommand) (contracts.ReservationResult, error) {
	var out contracts.ReservationResult
	if c.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.AssignmentID); err != nil {
		return out, err
	}
	hash := contracts.PayloadHash(c)
	var stored string
	var data []byte
	err = tx.QueryRow(ctx, `UPDATE reservation_results SET durability_counter=durability_counter+1 WHERE assignment_id=$1 RETURNING payload_hash,result`, c.AssignmentID).Scan(&stored, &data)
	if err == nil {
		if stored != hash {
			return out, confirmConflict(ctx, tx)
		}
		if err = json.Unmarshal(data, &out); err != nil {
			return out, err
		}
		return out, commit(ctx, tx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	tag, err := tx.Exec(ctx, `UPDATE resources SET status='ALLOCATED',availability=false,assigned_request_id=$2,version=version+1 WHERE id=$1 AND availability AND status='AVAILABLE' AND assigned_request_id IS NULL`, c.ResourceID, c.RequestID)
	if err != nil {
		return out, err
	}
	out = contracts.ReservationResult{AssignmentID: c.AssignmentID, RequestID: c.RequestID, ResourceID: c.ResourceID, Status: "REJECTED", Reason: "NOT_AVAILABLE"}
	if tag.RowsAffected() == 1 {
		out.Status = "SUCCEEDED"
		out.Reason = ""
	}
	data, err = json.Marshal(out)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reservation_results(assignment_id,resource_id,request_id,payload_hash,result) VALUES($1,$2,$3,$4,$5)`, c.AssignmentID, c.ResourceID, c.RequestID, hash, data); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}
func (s *Store) BeginIntent(ctx context.Context, c contracts.IntentCommand) (contracts.Intent, error) {
	out := contracts.Intent{}
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
	var hash string
	err = tx.QueryRow(ctx, `UPDATE assignment_attempts SET durability_counter=durability_counter+1 WHERE assignment_id=$1 RETURNING request_id,resource_id,generation,distance_meters,phase,payload_hash`, c.AssignmentID).Scan(&out.RequestID, &out.ResourceID, &out.Generation, &out.DistanceMeters, &out.Phase, &hash)
	if err == nil {
		out.AssignmentID = c.AssignmentID
		if hash != contracts.PayloadHash(c) {
			return out, confirmConflict(ctx, tx)
		}
		return out, commit(ctx, tx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	request, err := scan(tx.QueryRow(ctx, selectRequest+" FOR UPDATE", c.RequestID))
	if err != nil {
		return out, err
	}
	if request.Generation != c.Generation || request.Status != "PENDING" {
		if _, err = tx.Exec(ctx, `UPDATE requests SET version=version+1 WHERE id=$1`, c.RequestID); err != nil {
			return out, err
		}
		return out, confirmConflict(ctx, tx)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO assignment_attempts(assignment_id,request_id,resource_id,generation,distance_meters,payload_hash,phase) VALUES($1,$2,$3,$4,$5,$6,'INTENT')`, c.AssignmentID, c.RequestID, c.ResourceID, c.Generation, c.DistanceMeters, contracts.PayloadHash(c)); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `UPDATE requests SET status='ASSIGNING',version=version+1 WHERE id=$1`, c.RequestID); err != nil {
		return out, err
	}
	out = contracts.Intent{IntentCommand: c, Phase: "INTENT"}
	return out, commit(ctx, tx)
}
