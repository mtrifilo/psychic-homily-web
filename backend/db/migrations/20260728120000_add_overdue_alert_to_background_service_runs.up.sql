-- PSY-1612: throttle state for overdue-sweep alerting.
--
-- PSY-1611 made a loop's schedule durable, which is what lets anything answer
-- "has this stopped running?". This column is what stops that answer from
-- becoming noise: an overdue sweep STAYS overdue, so a health check that reports
-- on every pass floods Sentry until someone mutes it.
--
-- Policy (see internal/services/shared/overdue_check.go, which owns it): report
-- on the healthy -> overdue transition, re-assert at most once per window, clear
-- on the next successful cycle. NULL means "not currently in an alerted overdue
-- episode".
--
-- The timestamp lives in the run-state row, not in process memory, so the throttle
-- survives deploys and holds across replicas — the health check claims an alert
-- with an UPDATE ... RETURNING, so two instances cannot both report one
-- occurrence. Process-local state here would re-create the fault being monitored
-- for (PSY-1606).
--
-- ADDITIVE: one nullable column on a ~20-row table, no FKs, no backfill. Existing
-- rows start NULL, so any sweep already overdue reports on the first pass.

ALTER TABLE background_service_runs
    ADD COLUMN last_overdue_alert_at TIMESTAMPTZ;

-- Deliberately does not name the window length: it is configurable at runtime
-- (SWEEP_OVERDUE_REALERT_HOURS), and a column comment cannot be corrected in
-- place once deployed, so a hardcoded figure here would eventually be a lie told
-- by the schema itself.
COMMENT ON COLUMN background_service_runs.last_overdue_alert_at IS
    'PSY-1612: when this loop was last reported overdue. NULL = not in an alerted overdue episode; cleared on a successful cycle. Throttles re-alerting; window is set in application config.';
