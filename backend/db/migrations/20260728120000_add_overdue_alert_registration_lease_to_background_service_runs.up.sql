-- PSY-1612: throttle state for overdue-sweep alerting.
--
-- PSY-1611 made a loop's schedule durable, which is what lets anything answer
-- "has this stopped running?". This column is what stops that answer from
-- becoming noise: an overdue sweep STAYS overdue, so a health check that reports
-- on every pass floods Sentry until someone mutes it.
--
-- Policy (see internal/services/shared/overdue_check.go, which owns it): report
-- on the healthy -> overdue transition, re-assert at most once per window, and
-- clear when a cycle COMPLETES -- success or failure alike, because completing is
-- what stops a loop being overdue (see Complete in run_state.go, which argues the
-- point). NULL means "not currently in an alerted overdue episode".
--
-- The timestamp lives in the run-state row, not in process memory, so the throttle
-- survives deploys and holds across replicas — the health check claims an alert
-- with an UPDATE ... RETURNING, so two instances cannot both report one
-- occurrence. Process-local state here would re-create the fault being monitored
-- for (PSY-1606).
--
-- ADDITIVE: three nullable columns on a ~20-row table, no FKs, no backfill.
-- Existing rows start NULL on all three.
--
-- ROLLOUT EXPECTATION -- read this before deploying. The health check re-stamps
-- last_registered_at for every loop THIS PROCESS OWNS as the first step of each
-- pass, including the boot pass. So an owned loop already past its threshold is
-- eligible microseconds later and reports on the FIRST pass. Given PSY-1606 found
-- seven starved sweeps, expect a burst on the first deploy and schedule
-- accordingly. What stays quiet is only rows nothing owns -- renamed loops, and
-- rows carried in from another environment by a database restore.

ALTER TABLE background_service_runs
    ADD COLUMN last_overdue_alert_at TIMESTAMPTZ,
    ADD COLUMN last_registered_at    TIMESTAMPTZ,
    ADD COLUMN lease_seconds         BIGINT;

-- Deliberately does not name the window length: it is configurable at runtime
-- (SWEEP_OVERDUE_REALERT_HOURS), so a figure written here would drift from the
-- deployed value and the schema would assert something false.
COMMENT ON COLUMN background_service_runs.last_overdue_alert_at IS
    'PSY-1612: when this loop was last reported overdue. NULL = not in an alerted overdue episode; cleared when a cycle completes. Throttles re-alerting; window is set in application config.';

-- last_registered_at is how a loop stays "expected". A loop announces itself at
-- boot AND the health check re-stamps every loop this process owns on its own
-- cadence, so recency here tracks "some live process still wires this up" rather
-- than "a deploy happened recently" -- if it tracked deploys, a process simply
-- staying up past the window would retire the whole fleet and silence alerting.
-- A row nobody re-registers therefore describes something no longer wired up:
-- renamed, switched off, or carried into this environment by a database restore
-- (psy-deploy-prod --with-db-restore copies stage into prod, and stage runs
-- sweeps prod does not). Without this,
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

-- Supersedes PSY-1611's table comment, which is what \d+ shows and which now
-- describes neither the code (it names shared.RunTickerLoop, since renamed to
-- RunScheduledLoop) nor the alerting (it says health reads last_success_at /
-- consecutive_failures; PSY-1612 shipped keyed on last_completed_at, and nothing
-- predicates on consecutive_failures -- see PSY-1620).
COMMENT ON TABLE background_service_runs IS
    'Durable per-loop run state for shared.RunScheduledLoop. Scheduling reads last_completed_at. Overdue alerting (PSY-1612) reads last_completed_at / created_at / last_registered_at; last_success_at and consecutive_failures are alert CONTEXT only - nothing predicates on them yet (PSY-1620).';
