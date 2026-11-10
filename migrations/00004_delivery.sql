-- +goose Up
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_pkey;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_request_id_generation_destination_key;
ALTER TABLE outbox_events ADD COLUMN retry_number integer NOT NULL DEFAULT 0 CHECK(retry_number>=0);
ALTER TABLE outbox_events ADD COLUMN delivery_id text;
UPDATE outbox_events SET delivery_id=encode(substring(sha256(convert_to('erm:v1','UTF8') || decode('00','hex') || convert_to('delivery','UTF8') || decode('00','hex') || convert_to(event_id || ':0:' || destination,'UTF8')) from 1 for 16),'hex');
ALTER TABLE outbox_events ALTER COLUMN delivery_id SET NOT NULL;
ALTER TABLE outbox_events ADD PRIMARY KEY(delivery_id);
ALTER TABLE outbox_events ADD UNIQUE(event_id,retry_number,destination);
ALTER TABLE outbox_events ADD COLUMN claim_token text;
ALTER TABLE outbox_events ADD COLUMN claim_until timestamptz;
ALTER TABLE outbox_events ADD COLUMN durability_counter bigint NOT NULL DEFAULT 0;
CREATE INDEX outbox_due ON outbox_events(due_at) WHERE sent_at IS NULL;

-- +goose Down
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_pkey;
ALTER TABLE outbox_events ADD PRIMARY KEY(event_id);
ALTER TABLE outbox_events ADD UNIQUE(request_id,generation,destination);
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_id_retry_number_destination_key;
DROP INDEX outbox_due;
ALTER TABLE outbox_events DROP COLUMN delivery_id, DROP COLUMN retry_number, DROP COLUMN claim_token, DROP COLUMN claim_until, DROP COLUMN durability_counter;
