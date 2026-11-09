-- +goose Up
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE TABLE requests (
 id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
 type text NOT NULL CHECK (type IN ('WATER','FOOD','MEDICINE','TRANSPORTATION','SHELTER')),
 location geography(Point,4326) NOT NULL,
 details text NOT NULL CHECK (char_length(details) BETWEEN 1 AND 2000),
 status text NOT NULL CHECK (status IN ('PENDING','ASSIGNING','ASSIGNED','UNMATCHED','COMPLETED')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 generation bigint NOT NULL DEFAULT 1 CHECK (generation>0),
 assigned_resource_id text,
 version bigint NOT NULL DEFAULT 1
);
CREATE INDEX requests_location_idx ON requests USING gist(location);
CREATE TABLE operations (
 entity_id text NOT NULL REFERENCES requests(id),
 kind text NOT NULL,
 operation_key text NOT NULL,
 payload_hash text NOT NULL,
 result jsonb NOT NULL,
 durability_counter bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(entity_id,kind,operation_key)
);
CREATE TABLE outbox_events (
 event_id text PRIMARY KEY,
 request_id text NOT NULL REFERENCES requests(id),
 generation bigint NOT NULL,
 payload jsonb NOT NULL,
 destination text NOT NULL DEFAULT 'request.match',
 due_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 sent_at timestamptz,
 UNIQUE(request_id,generation,destination)
);

-- +goose Down
DROP TABLE outbox_events;
DROP TABLE operations;
DROP TABLE requests;
