package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

var syncCanceled sync.Map

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.ConnConfig.OnNotice = func(c *pgconn.PgConn, n *pgconn.Notice) {
		if strings.Contains(n.Message, "canceling wait for synchronous replication") {
			syncCanceled.Store(c, true)
		}
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
func New(pool *pgxpool.Pool) *Store               { return &Store{pool: pool} }
func (s *Store) Health(ctx context.Context) error { return s.pool.Ping(ctx) }
func cleanup(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func commit(ctx context.Context, tx pgx.Tx) error {
	conn := tx.Conn().PgConn()
	syncCanceled.Delete(conn)
	err := tx.Commit(ctx)
	_, warning := syncCanceled.LoadAndDelete(conn)
	if err != nil || ctx.Err() != nil || warning {
		return contracts.ErrUnavailable
	}
	return nil
}

func confirmConflict(ctx context.Context, tx pgx.Tx) error {
	if err := commit(ctx, tx); err != nil {
		return err
	}
	return contracts.ErrConflict
}

func (s *Store) Create(ctx context.Context, key string, c contracts.CreateRequest) (contracts.Request, error) {
	var out contracts.Request
	if contracts.ValidateKey(key) != nil || c.Validate() != nil {
		return out, contracts.ErrInvalid
	}
	id := contracts.EntityID("create-request", key)
	hash := contracts.PayloadHash(c)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	// The transaction lock serializes replay/conflict checks for this aggregate.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return out, err
	}
	found, err := replay(ctx, tx, id, "create-request", key, hash, &out)
	if errors.Is(err, contracts.ErrConflict) {
		return out, confirmConflict(ctx, tx)
	}
	if err != nil {
		return out, err
	}
	if found {
		return out, commit(ctx, tx)
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM requests WHERE id=$1)", id).Scan(&exists); err != nil {
		return out, err
	}
	if exists {
		if _, err = tx.Exec(ctx, "UPDATE requests SET version=version+1 WHERE id=$1", id); err != nil {
			return out, err
		}
		return out, confirmConflict(ctx, tx)
	}
	out = contracts.Request{ID: id, Type: c.Type, Location: c.Location, Details: c.Details, Status: "PENDING", Generation: 1}
	err = tx.QueryRow(ctx, `INSERT INTO requests(id,type,location,details,status) VALUES($1,$2,ST_SetSRID(ST_MakePoint($3,$4),4326)::geography,$5,'PENDING') RETURNING created_at`, id, c.Type, *c.Location.Longitude, *c.Location.Latitude, c.Details).Scan(&out.CreatedAt)
	if err != nil {
		return out, err
	}
	if err = record(ctx, tx, id, "create-request", key, hash, out); err != nil {
		return out, err
	}
	eventID := contracts.EntityID("match-event", id+":1")
	payload, _ := json.Marshal(map[string]any{"schemaVersion": 1, "eventId": eventID, "requestId": id, "generation": 1, "createdAt": out.CreatedAt})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(event_id,request_id,generation,payload) VALUES($1,$2,1,$3)`, eventID, id, payload); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}

func replay(ctx context.Context, tx pgx.Tx, id, kind, key, hash string, out *contracts.Request) (bool, error) {
	var stored string
	var data []byte
	err := tx.QueryRow(ctx, `SELECT payload_hash,result FROM operations WHERE entity_id=$1 AND kind=$2 AND operation_key=$3 FOR UPDATE`, id, kind, key).Scan(&stored, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// A real write forces a fresh synchronous durability acknowledgement on replay.
	_, err = tx.Exec(ctx, `UPDATE operations SET durability_counter=durability_counter+1 WHERE entity_id=$1 AND kind=$2 AND operation_key=$3`, id, kind, key)
	if err != nil {
		return false, err
	}
	if stored != hash {
		return true, contracts.ErrConflict
	}
	if err = json.Unmarshal(data, out); err != nil {
		return false, err
	}
	return true, err
}
func record(ctx context.Context, tx pgx.Tx, id, kind, key, hash string, out contracts.Request) error {
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO operations(entity_id,kind,operation_key,payload_hash,result) VALUES($1,$2,$3,$4,$5)`, id, kind, key, hash, data)
	return err
}

const selectRequest = `SELECT id,type,ST_Y(location::geometry),ST_X(location::geometry),details,status,created_at,generation,coalesce(assigned_resource_id,'') FROM requests WHERE id=$1`

func scan(row pgx.Row) (contracts.Request, error) {
	var r contracts.Request
	var lat, lon float64
	err := row.Scan(&r.ID, &r.Type, &lat, &lon, &r.Details, &r.Status, &r.CreatedAt, &r.Generation, &r.AssignedResourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, contracts.ErrNotFound
	}
	r.Location = &contracts.Location{Latitude: &lat, Longitude: &lon}
	return r, err
}
func (s *Store) Get(ctx context.Context, id string) (contracts.Request, error) {
	return scan(s.pool.QueryRow(ctx, selectRequest, id))
}
func (s *Store) SetStatus(ctx context.Context, id, key, status string) (contracts.Request, error) {
	var out contracts.Request
	if contracts.ValidateKey(key) != nil {
		return out, contracts.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return out, err
	}
	hash := contracts.PayloadHash(contracts.StatusCommand{Status: status})
	found, err := replay(ctx, tx, id, "request-status", key, hash, &out)
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
	if status != out.Status && !(out.Status == "ASSIGNED" && status == "COMPLETED") {
		if _, err = tx.Exec(ctx, "UPDATE requests SET version=version+1 WHERE id=$1", id); err != nil {
			return out, err
		}
		return out, confirmConflict(ctx, tx)
	}
	out.Status = status
	if _, err = tx.Exec(ctx, "UPDATE requests SET status=$2,version=version+1 WHERE id=$1", id, status); err != nil {
		return out, err
	}
	if err = record(ctx, tx, id, "request-status", key, hash, out); err != nil {
		return out, err
	}
	return out, commit(ctx, tx)
}
