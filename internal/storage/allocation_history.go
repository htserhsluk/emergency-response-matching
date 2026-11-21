package storage

import (
	"context"
	"errors"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/jackc/pgx/v5"
)

func (s *Store) AllocationConfirmation(ctx context.Context, resource, request string) (time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx, `SELECT confirmed_at FROM legacy_allocation_confirmations WHERE resource_id=$1 AND request_id=$2`, resource, request).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return at, err
}

func (s *Store) allocationHistory(ctx context.Context, q contracts.ListQuery) (contracts.ResourcePage, error) {
	// The two indexed reads have independent shard ownership. Only the global
	// page needs to resolve locations from legacy request-side confirmations.
	predicates := ""
	args := []any{contracts.AllocationHistoryLimit}
	if q.Type != "" {
		predicates = " AND type=$2"
		args = append(args, q.Type)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+resourceColumns+` FROM resources WHERE status='ALLOCATED'`+predicates+` ORDER BY allocated_at DESC NULLS LAST,id DESC LIMIT $1`, args...)
	if err != nil {
		return contracts.ResourcePage{}, err
	}
	current, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (contracts.Resource, error) { return scanResource(row) })
	if err != nil {
		return contracts.ResourcePage{}, err
	}
	rows, err = s.pool.Query(ctx, `SELECT resource_id,request_id,type,confirmed_at FROM legacy_allocation_confirmations WHERE true`+predicates+` ORDER BY confirmed_at DESC,resource_id DESC LIMIT $1`, args...)
	if err != nil {
		return contracts.ResourcePage{}, err
	}
	legacy, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (contracts.Resource, error) {
		r := contracts.Resource{Status: "ALLOCATED"}
		err := row.Scan(&r.ID, &r.AssignedRequestID, &r.Type, &r.AllocationConfirmedAt)
		return r, err
	})
	if err != nil {
		return contracts.ResourcePage{}, err
	}
	return contracts.AllocationPage(append(current, legacy...), q), nil
}
