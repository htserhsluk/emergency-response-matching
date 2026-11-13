-- +goose Up
CREATE TABLE notifications (
 id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
 request_id text NOT NULL REFERENCES requests(id),
 assignment_id text NOT NULL REFERENCES assignment_attempts(assignment_id),
 resource_id text NOT NULL CHECK (resource_id ~ '^[0-9a-f]{32}$'),
 recipient text NOT NULL CHECK (recipient='demo'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 read_at timestamptz,
 durability_counter bigint NOT NULL DEFAULT 0,
 UNIQUE(assignment_id,recipient)
);
CREATE INDEX notification_request_page ON notifications(request_id,id);
-- Historical assignments receive an inbox record at migration time. Its
-- timestamp is the inbox creation time, not an invented assignment timestamp.
INSERT INTO notifications(id,request_id,assignment_id,resource_id,recipient)
 SELECT assignment_id,request_id,assignment_id,resource_id,'demo'
 FROM assignment_attempts WHERE phase='FINALIZED';

-- +goose Down
DROP TABLE notifications;
