-- Reverses PSY-1612's alert-throttle column.
--
-- Dropping it loses only throttle state, never schedule state: scheduling reads
-- last_completed_at, which this migration never touched. The cost of a down/up
-- cycle is therefore at most one duplicate overdue report per stalled loop.

ALTER TABLE background_service_runs
    DROP COLUMN IF EXISTS last_overdue_alert_at;
