-- +goose Up
CREATE TABLE processed_events (
 event_id text PRIMARY KEY,
 request_id text NOT NULL REFERENCES requests(id),
 generation bigint NOT NULL,
 state text NOT NULL CHECK(state IN ('RUNNING','PAUSED','DONE')),
 expected_retry integer NOT NULL DEFAULT 0 CHECK(expected_retry>=0),
 retry_limit integer NOT NULL DEFAULT 3 CHECK(retry_limit>=0),
 due_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 last_error text NOT NULL DEFAULT '',
 durability_counter bigint NOT NULL DEFAULT 0,
 UNIQUE(request_id,generation)
);
CREATE INDEX workflow_due ON processed_events(due_at) WHERE state='RUNNING';
-- +goose Down
DROP TABLE processed_events;
