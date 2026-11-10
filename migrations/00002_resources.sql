-- +goose Up
-- Operations are owned by either a request or a resource on this same shard.
ALTER TABLE operations DROP CONSTRAINT operations_entity_id_fkey;
CREATE TABLE resources (
 id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
 type text NOT NULL CHECK (type IN ('WATER','FOOD','MEDICINE','TRANSPORTATION','SHELTER')),
 location geography(Point,4326) NOT NULL,
 availability boolean NOT NULL,
 status text NOT NULL CHECK (status IN ('AVAILABLE','UNAVAILABLE','ALLOCATED')),
 assigned_request_id text CHECK (assigned_request_id ~ '^[0-9a-f]{32}$'),
 version bigint NOT NULL DEFAULT 1,
 CHECK (availability = (status='AVAILABLE')),
 CHECK ((status='ALLOCATED') = (assigned_request_id IS NOT NULL))
);
CREATE INDEX resources_available_location ON resources USING gist(location) WHERE availability;
CREATE INDEX resources_type ON resources(type) WHERE availability;

-- +goose Down
-- Refuse a destructive downgrade if resource history exists.
ALTER TABLE operations ADD CONSTRAINT operations_entity_id_fkey FOREIGN KEY(entity_id) REFERENCES requests(id);
DROP TABLE resources;
