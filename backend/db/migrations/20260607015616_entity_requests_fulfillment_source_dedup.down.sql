-- PSY-1008: reverse the fulfillment-outcome / source-context / dedup additions.
-- Order: drop the index first, then the columns. IF EXISTS guards keep the
-- down→up round-trip (CI's migrate down -all → up) idempotent.

DROP INDEX IF EXISTS uq_entity_requests_pending_dedup;

ALTER TABLE entity_requests DROP COLUMN IF EXISTS source_detail;
ALTER TABLE entity_requests DROP COLUMN IF EXISTS created_entity_id;
