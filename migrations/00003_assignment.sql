-- +goose Up
CREATE TABLE reservation_results (
 assignment_id text PRIMARY KEY CHECK (assignment_id ~ '^[0-9a-f]{32}$'),
 resource_id text NOT NULL CHECK (resource_id ~ '^[0-9a-f]{32}$'),
 request_id text NOT NULL CHECK (request_id ~ '^[0-9a-f]{32}$'),
 payload_hash text NOT NULL,
 result jsonb NOT NULL,
 durability_counter bigint NOT NULL DEFAULT 0
);
CREATE INDEX reservation_resource_idx ON reservation_results(resource_id);
CREATE TABLE assignment_attempts (
 assignment_id text PRIMARY KEY CHECK (assignment_id ~ '^[0-9a-f]{32}$'),
 request_id text NOT NULL REFERENCES requests(id),
 resource_id text NOT NULL CHECK (resource_id ~ '^[0-9a-f]{32}$'),
 generation bigint NOT NULL CHECK (generation>0),
 distance_meters double precision NOT NULL CHECK(distance_meters>=0),
 payload_hash text NOT NULL,
 phase text NOT NULL CHECK (phase IN ('INTENT','RESERVED','FINALIZED','REJECTED')),
 durability_counter bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX one_unresolved_intent ON assignment_attempts(request_id) WHERE phase IN ('INTENT','RESERVED','FINALIZED');

-- +goose Down
DROP TABLE assignment_attempts;
DROP TABLE reservation_results;
