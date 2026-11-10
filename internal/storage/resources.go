package storage

import (
	"context"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"github.com/jackc/pgx/v5"
)

const resourceColumns = `id,type,ST_Y(location::geometry),ST_X(location::geometry),availability,status,coalesce(assigned_request_id,'')`

func scanResource(row pgx.Row) (contracts.Resource, error) {
	var r contracts.Resource
	var lat, lon float64
	err := row.Scan(&r.ID, &r.Type, &lat, &lon, &r.Availability, &r.Status, &r.AssignedRequestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, contracts.ErrNotFound
	}
	r.Location = &contracts.Location{Latitude: &lat, Longitude: &lon}
	return r, err
}
func (s *Store) GetResource(ctx context.Context, id string) (contracts.Resource, error) {
	if _, err := routing.ShardFor(id); err != nil {
		return contracts.Resource{}, contracts.ErrInvalid
	}
	return scanResource(s.pool.QueryRow(ctx, `SELECT `+resourceColumns+` FROM resources WHERE id=$1`, id))
}
func (s *Store) RegisterResource(ctx context.Context, key string, c contracts.RegisterResource) (contracts.Resource, error) {
	var out contracts.Resource
	if contracts.ValidateKey(key) != nil || c.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	id := contracts.EntityID("register-resource", key)
	hash := contracts.PayloadHash(c)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id); err != nil {
		return out, err
	}
	found, err := replay(ctx, tx, id, "register-resource", key, hash, &out)
	if errors.Is(err, contracts.ErrConflict) {
		return out, confirmConflict(ctx, tx)
	}
	if err != nil {
		return out, err
	}
	if found {
		return out, commit(ctx, tx)
	}
	existing, err := tx.Exec(ctx, `UPDATE resources SET version=version+1 WHERE id=$1`, id)
	if err != nil {
		return out, err
	}
	if existing.RowsAffected() != 0 {
		return out, confirmConflict(ctx, tx)
	}
	status := "UNAVAILABLE"
	if *c.Availability {
		status = "AVAILABLE"
	}
	out, err = scanResource(tx.QueryRow(ctx, `INSERT INTO resources(id,type,location,availability,status) VALUES($1,$2,ST_SetSRID(ST_MakePoint($3,$4),4326)::geography,$5,$6) RETURNING `+resourceColumns, id, c.Type, *c.Location.Longitude, *c.Location.Latitude, *c.Availability, status))
	if err != nil {
		return out, err
	}
	if err = record(ctx, tx, id, "register-resource", key, hash, out); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}
func (s *Store) SetResourceStatus(ctx context.Context, id, key, status string) (contracts.Resource, error) {
	var out contracts.Resource
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
	hash := contracts.PayloadHash(contracts.StatusCommand{Status: status})
	found, err := replay(ctx, tx, id, "resource-status", key, hash, &out)
	if errors.Is(err, contracts.ErrConflict) {
		return out, confirmConflict(ctx, tx)
	}
	if err != nil {
		return out, err
	}
	if found {
		return out, commit(ctx, tx)
	}
	out, err = scanResource(tx.QueryRow(ctx, `UPDATE resources SET version=version+1 WHERE id=$1 RETURNING `+resourceColumns, id))
	if err != nil {
		return out, err
	}
	if (status != "AVAILABLE" && status != "UNAVAILABLE") || out.Status == "ALLOCATED" {
		return out, confirmConflict(ctx, tx)
	}
	out, err = scanResource(tx.QueryRow(ctx, `UPDATE resources SET status=$2,availability=($2='AVAILABLE') WHERE id=$1 RETURNING `+resourceColumns, id, status))
	if err != nil {
		return out, err
	}
	if err = record(ctx, tx, id, "resource-status", key, hash, out); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}
func (s *Store) ListResources(ctx context.Context, q contracts.ListQuery) (contracts.ResourcePage, error) {
	out := contracts.ResourcePage{Items: []contracts.Resource{}}
	if q.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	rows, err := s.pool.Query(ctx, `SELECT `+resourceColumns+` FROM resources WHERE id>$1 ORDER BY id LIMIT $2`, q.After, q.Limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanResource(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, r)
		out.Next = r.ID
	}
	return out, rows.Err()
}
func (s *Store) Search(ctx context.Context, q contracts.SearchQuery) ([]contracts.Candidate, error) {
	if q.Validate() != nil {
		return nil, contracts.ErrInvalid
	}
	rows, err := s.pool.Query(ctx, `WITH nearby AS (SELECT id,ST_Distance(location,ST_SetSRID(ST_MakePoint($1,$2),4326)::geography) AS distance FROM resources WHERE availability AND type=$3 AND ST_DWithin(location,ST_SetSRID(ST_MakePoint($1,$2),4326)::geography,$4)) SELECT id,distance FROM nearby WHERE ($5='' OR (distance,id)>($6,$5)) ORDER BY distance,id LIMIT $7`, *q.Location.Longitude, *q.Location.Latitude, q.Type, q.RadiusMeters, q.AfterID, q.AfterDistance, q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Candidate{}
	for rows.Next() {
		var c contracts.Candidate
		if err := rows.Scan(&c.ResourceID, &c.DistanceMeters); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
