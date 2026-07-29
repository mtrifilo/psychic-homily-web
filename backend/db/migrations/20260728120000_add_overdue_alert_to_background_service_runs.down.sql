-- Reverses PSY-1612's alert-throttle column.
--
-- Drops only throttle state, never schedule state: scheduling reads
-- last_completed_at, which this migration never touched. The cost of a down/up
-- cycle is at most one duplicate overdue report per stalled loop.

ALTER TABLE background_service_runs
    DROP COLUMN IF EXISTS last_overdue_alert_at,
    DROP COLUMN IF EXISTS last_registered_at,
    DROP COLUMN IF EXISTS lease_seconds;
