-- +goose Up
ALTER TABLE processed_events ADD COLUMN search_radius integer NOT NULL DEFAULT 0 CHECK(search_radius BETWEEN 0 AND 14);
ALTER TABLE processed_events ADD COLUMN search_after_id text NOT NULL DEFAULT '';
ALTER TABLE processed_events ADD COLUMN search_after_distance double precision NOT NULL DEFAULT 0 CHECK(search_after_distance >= 0);

-- +goose Down
-- Refuse downgrade: dropping continuation could discard a live search position.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Search progress downgrade requires an explicit migration plan'; END $$;
-- +goose StatementEnd
