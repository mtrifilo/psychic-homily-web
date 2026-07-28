-- PSY-1611: background_service_runs — durable run-state for periodic background
-- loops.
--
-- Why this table exists: shared.RunTickerLoop anchored its ticker at PROCESS
-- START, so a loop whose interval exceeded the deploy cadence never reached its
-- first tick (PSY-1606: seven production sweeps ran exactly one cycle, ever).
-- Persisting when a loop last completed lets the next boot decide "am I overdue?"
-- instead of restarting a timer that never finishes. Deploys stop mattering.
--
-- The row is also the run-TRACE: outcome, error, duration and rows processed are
-- the substrate PSY-1612's overdue alerting reads, and the reason radio_janitor
-- (whose every write is shared with a 6h loop) becomes verifiable at all.
--
-- One row per loop name, so this table stays at ~20 rows forever. No history is
-- kept deliberately — a journal would need retention management to answer a
-- question ("is this loop healthy right now?") that the latest row already
-- answers.
--
-- ADDITIVE: one new table, no FKs (in particular none to `users`, so no
-- test-fixtures allowlist entry is required). Multi-statement => golang-migrate
-- wraps it in a transaction => no CREATE INDEX CONCURRENTLY here.

CREATE TABLE background_service_runs (
    -- The loop name passed to shared.RunTickerLoop. PK because there is exactly
    -- one live state per loop; the natural key beats a surrogate id here since
    -- every read and every claim is by name.
    name TEXT PRIMARY KEY,

    -- The interval the loop was LAST configured with, in seconds. Stored so
    -- overdue-detection (PSY-1612) can be answered from data alone rather than
    -- by cross-referencing env vars against source code — the exact
    -- config-inspection dependency that made PSY-1606 expensive to diagnose.
    interval_seconds BIGINT NOT NULL,

    -- Claim timestamp. Written when a cycle starts and left in place while it
    -- runs, so a concurrent instance can tell a live run from an abandoned one.
    last_started_at TIMESTAMPTZ,

    -- Terminal timestamp for the last cycle, written for BOTH success and
    -- failure. This is what SCHEDULING keys on: a failing loop must retry on its
    -- normal cadence rather than reading as permanently overdue and firing a
    -- third-party sweep on every single deploy.
    last_completed_at TIMESTAMPTZ,

    -- Last cycle that actually succeeded. Split from last_completed_at so health
    -- ("has this worked lately?") is separable from schedule ("when is it next
    -- due?"). PSY-1612 alerts on this one.
    last_success_at TIMESTAMPTZ,

    -- 'running' | 'success' | 'error' | 'panic'. Deliberately a TEXT check rather
    -- than an enum type: outcomes are a volatile vocabulary and a CHECK is far
    -- cheaper to extend than an ALTER TYPE across environments.
    last_outcome TEXT CHECK (last_outcome IN ('running', 'success', 'error', 'panic')),

    -- Truncated by the writer; a runaway error string must not bloat the row.
    last_error TEXT,
    last_duration_ms BIGINT,

    -- NULL means "the cycle did not report a count", which is different from 0
    -- ("it ran and found nothing to do") — PSY-1613 needs to tell those apart.
    last_rows_processed BIGINT,

    -- Reset to 0 on success. The alerting signal for "failing every cycle but
    -- never going overdue", which last_completed_at alone would hide.
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    run_count BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE background_service_runs IS
    'PSY-1611: durable per-loop run state for shared.RunTickerLoop. Scheduling reads last_completed_at; health reads last_success_at / consecutive_failures.';
