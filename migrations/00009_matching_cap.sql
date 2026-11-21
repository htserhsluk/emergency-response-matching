-- +goose Up
-- Schema 7 indices 0..10 have identical radii. Index 11 formerly covered
-- 5,000 km. Keep its cursor while it is inside the new last radius. Later
-- indices have already completed all candidates within the cap.
UPDATE processed_events SET search_radius=12,search_after_id='',search_after_distance=0
WHERE search_radius>11 OR (search_radius=11 AND search_after_distance>4000000);
ALTER TABLE processed_events ADD CONSTRAINT matching_radius_index CHECK(search_radius <= 12);

ALTER TABLE requests ADD COLUMN availability_checked_at timestamptz;
CREATE INDEX requests_unmatched_rotation ON requests((coalesce(availability_checked_at,created_at)),id)
WHERE status='UNMATCHED';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Matching policy downgrade requires an explicit migration plan'; END $$;
-- +goose StatementEnd
