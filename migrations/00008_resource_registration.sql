-- +goose Up
-- Historical resources and their registration operations have no recorded time.
-- The zero timestamp groups those rows behind all dated registrations.
ALTER TABLE resources ADD COLUMN registered_at timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00+00';
ALTER TABLE resources ALTER COLUMN registered_at SET DEFAULT clock_timestamp();
CREATE INDEX resources_registered_id ON resources (registered_at DESC, id DESC);

-- +goose Down
ALTER TABLE resources DROP COLUMN registered_at;
