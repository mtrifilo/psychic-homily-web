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
    ADD COLUMN last_overdue_alert_at TIMESTAMPTZ,
    ADD COLUMN last_registered_at    TIMESTAMPTZ,
    ADD COLUMN lease_seconds         BIGINT;

-- Deliberately does not name the window length: it is configurable at runtime
-- (SWEEP_OVERDUE_REALERT_HOURS), and a column comment cannot be corrected in
-- place once deployed, so a hardcoded figure here would eventually be a lie told
-- by the schema itself.
COMMENT ON COLUMN background_service_runs.last_overdue_alert_at IS
    'PSY-1612: when this loop was last reported overdue. NULL = not in an alerted overdue episode; cleared when a cycle completes. Throttles re-alerting; window is set in application config.';

-- last_registered_at is how a loop stays "expected". A loop announces itself on
-- every boot, so a row nobody has re-registered for a long time describes
-- something that is no longer wired up: renamed, switched off, or carried into
-- this environment by a database restore (psy-deploy-prod --with-db-restore
-- copies stage into prod, and stage runs sweeps prod does not). Without this,
-- such a row is indistinguishable from a stalled sweep and alerts every day
-- forever with no in-app way to clear it — the precise alert fatigue this
-- feature exists to avoid.
--
-- It also means a loop genuinely dropped from wiring by accident still alerts
-- for the whole retirement window first, which is ample time to notice.
COMMENT ON COLUMN background_service_runs.last_registered_at IS
    'PSY-1612: last time a running process declared this loop exists. Stale => the loop is retired, not stalled, and is excluded from overdue alerting.';

-- lease_seconds is stored for the same reason interval_seconds is: so overdue
-- detection is answerable from DATA rather than by cross-referencing source
-- code. The overdue threshold must clear one interval PLUS one lease, because a
-- process killed mid-cycle leaves a claim that blocks re-claiming for a full
-- lease — for an hourly loop with an hourly lease, a bare 2x interval threshold
-- goes true at the exact moment recovery completes, producing a false page from
-- ordinary platform behaviour.
COMMENT ON COLUMN background_service_runs.lease_seconds IS
    'PSY-1612: the claim lease this loop was last configured with. Feeds the overdue threshold floor so a crash-recovery gap cannot read as a stall.';
