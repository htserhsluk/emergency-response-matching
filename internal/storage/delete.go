package storage

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
)

func (s *Store) DeleteData(ctx context.Context, kind, id string) error {
	if _, err := routing.ShardFor(id); err != nil {
		return contracts.ErrInvalid
	}
	if kind != "request" && kind != "resource" {
		return contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id); err != nil {
		return err
	}
	// Whitelisted table names only. A row lock also serializes resource reservation.
	table := "requests"
	history := `SELECT EXISTS(SELECT 1 FROM operations WHERE entity_id=$1 UNION ALL SELECT 1 FROM assignment_attempts WHERE request_id=$1 UNION ALL SELECT 1 FROM outbox_events WHERE request_id=$1 UNION ALL SELECT 1 FROM reservation_results WHERE request_id=$1)`
	if kind == "resource" {
		table = "resources"
		history = `SELECT EXISTS(SELECT 1 FROM operations WHERE entity_id=$1 UNION ALL SELECT 1 FROM reservation_results WHERE resource_id=$1 UNION ALL SELECT 1 FROM assignment_attempts WHERE resource_id=$1)`
	}
	tag, err := tx.Exec(ctx, `UPDATE `+table+` SET version=version+1 WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return contracts.ErrNotFound
	}
	var protected bool
	if err = tx.QueryRow(ctx, history, id).Scan(&protected); err != nil {
		return err
	}
	if protected {
		return confirmConflict(ctx, tx)
	}
	if kind == "resource" {
		if err = tx.QueryRow(ctx, `SELECT status='ALLOCATED' FROM resources WHERE id=$1`, id).Scan(&protected); err != nil {
			return err
		}
		if protected {
			return confirmConflict(ctx, tx)
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM `+table+` WHERE id=$1`, id); err != nil {
		return err
	}
	return commit(ctx, tx)
}
