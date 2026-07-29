-- Reverses PSY-1612's alert-throttle column.
--
-- Scheduling is unaffected: it reads last_completed_at, which this migration
-- never touched, so no sweep changes cadence.
--
-- ALERTING, however, goes fully dark until the next boot. Re-applying the up
-- migration leaves last_registered_at NULL for every row, and NULL reads as
-- retired, so no loop is reported until a process registers it again. Restart the
-- API after a down/up, or overdue alerting stays silent while looking healthy.

ALTER TABLE background_service_runs
    DROP COLUMN IF EXISTS last_overdue_alert_at,
    DROP COLUMN IF EXISTS last_registered_at,
    DROP COLUMN IF EXISTS lease_seconds;
