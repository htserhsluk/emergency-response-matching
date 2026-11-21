-- +goose Up
ALTER TABLE resources ADD COLUMN allocated_at timestamptz;
CREATE INDEX resources_allocation_order ON resources(allocated_at DESC NULLS LAST,id DESC) WHERE status='ALLOCATED';
CREATE INDEX resources_type_allocation_order ON resources(type,allocated_at DESC NULLS LAST,id DESC) WHERE status='ALLOCATED';

-- Before schema 10 reservation timestamps were not captured. Preserve evidence
-- of confirmation on its owning request shard. Schema 6 also created inbox
-- rows for older assignments, so confirmation time can postdate allocation.
CREATE TABLE legacy_allocation_confirmations AS
 SELECT n.resource_id,n.request_id,r.type,n.created_at AS confirmed_at
 FROM notifications n JOIN requests r ON r.id=n.request_id
 WHERE r.assigned_resource_id=n.resource_id AND r.status IN ('ASSIGNED','COMPLETED');
ALTER TABLE legacy_allocation_confirmations ADD PRIMARY KEY(resource_id);
CREATE INDEX legacy_allocation_order ON legacy_allocation_confirmations(confirmed_at DESC,resource_id DESC);
CREATE INDEX legacy_type_allocation_order ON legacy_allocation_confirmations(type,confirmed_at DESC,resource_id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Downgrade requires preserving recorded allocation timestamps'; END $$;
-- +goose StatementEnd
