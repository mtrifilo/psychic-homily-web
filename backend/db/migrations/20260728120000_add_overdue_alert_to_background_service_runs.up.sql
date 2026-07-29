-- PSY-1612: throttle state for overdue-sweep alerting.
--
-- PSY-1611 made a loop's schedule durable, which is what lets anything answer
-- "has this stopped running?". This column is what stops that answer from
-- becoming noise.
--
-- An overdue sweep STAYS overdue, so a health check that reports on every pass
-- floods Sentry until someone mutes it — and a muted alert is worse than no
-- alert, because it looks like coverage. The chosen policy is: report once on
-- the healthy -> overdue transition, then re-assert at most once every 24h while
-- it remains overdue, and clear on the next successful cycle.
--
-- Keeping that timestamp in the same row as the run state is what makes the
-- policy correct across replicas: the health check claims an alert with an
-- UPDATE ... WHERE last_overdue_alert_at IS NULL OR ... RETURNING, so two
-- instances checking simultaneously cannot both report the same occurrence.
-- Holding it in process memory instead would re-alert once per replica and reset
-- on every deploy — the same process-local-state mistake that caused PSY-1606.
--
-- NULL means "not currently in an alerted overdue episode". Complete() resets it
-- to NULL on a successful cycle, so recovery followed by a later failure reads as
-- a fresh transition and reports immediately rather than waiting out the window.
--
-- ADDITIVE: one nullable column on a ~20-row table, no FKs, no backfill. Existing
-- rows start NULL, i.e. any sweep already overdue when this deploys reports on the
-- first health-check pass.

ALTER TABLE background_service_runs
    ADD COLUMN last_overdue_alert_at TIMESTAMPTZ;

COMMENT ON COLUMN background_service_runs.last_overdue_alert_at IS
    'PSY-1612: when this loop was last reported overdue. NULL = not in an alerted overdue episode; cleared on a successful cycle. Throttles re-alerting to once per 24h.';
