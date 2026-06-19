-- PSY-1132: Add radio observability schema — radio_sync_runs,
-- radio_sync_run_errors, radio_station_health. Phase 1 (P1) of the greenfield
-- Radio Ingestion Redesign; the observability backbone every later phase reads.
--
-- ADDITIVE ONLY. These are three brand-new tables; nothing existing is touched.
-- The write path that POPULATES them is P2 (RunStationSync); the admin surfaces
-- that read them are P5. radio_import_jobs is intentionally left intact here — it
-- is retired into radio_sync_runs in P2, not dropped in P1.
--
-- Enum representation: CHECK constraints (not Postgres enum TYPEs), matching the
-- P1 entity-schema decision (20260619020546) — during a greenfield rebuild every
-- value set is still volatile, and a CHECK is trivially reversible / cheap to
-- evolve whereas an enum TYPE cannot drop values or be dropped while referenced.
--
-- Multi-statement file => golang-migrate runs it in a transaction => no CREATE
-- INDEX CONCURRENTLY (illegal in a txn, and unnecessary on empty new tables).

-- ============================================================================
-- radio_sync_runs — the observability backbone.
-- ============================================================================
-- Every ingestion path (scheduled, manual, auto-backfill) writes ONE row here so
-- a run leaves a durable, queryable trace instead of evaporating into logs (the
-- synchronous /import path persists nothing today; KEXP returned 0 plays for
-- weeks with no per-run signal — PSY-1126). One row is opened with status
-- 'running' at the START of a run (§4 step 1) so a mid-run crash leaves an
-- observable 'running' row rather than no record at all.
--
-- This table UNIFIES and replaces radio_import_jobs in P2 (locked decision §9.3),
-- so it carries forward that table's load-bearing fields: the requested historic
-- window (window_start/window_end <- the old since/until), live/terminal status
-- including 'cancelled' (an in-flight backfill is abortable), and the
-- current_episode_date progress marker the async UI polls. (PSY-1132 amends the
-- design doc §3.2 enum to add 'running' + 'cancelled' and the window columns, so
-- admin-triggered historic backfill stays both parameterizable AND observable in
-- the unified model — confirmed with the owner.)
CREATE TABLE radio_sync_runs (
    id SERIAL PRIMARY KEY,
    -- The station this run operated on. Operational history dies with the entity
    -- it describes (it is volatile operational state, not a durable record), so
    -- ON DELETE CASCADE — and a station hard-delete is never blocked by run rows.
    station_id INTEGER NOT NULL REFERENCES radio_stations(id) ON DELETE CASCADE,
    -- Nullable: set for a show-scoped manual import/backfill, NULL for a
    -- station-wide discover/fetch. ON DELETE SET NULL keeps the run history when a
    -- show is hard-deleted (the run is station-scoped; only the show link is lost).
    show_id INTEGER REFERENCES radio_shows(id) ON DELETE SET NULL,
    -- What the run did. discover = enumerate the provider roster; fetch = pull new
    -- episodes; backfill = re-ingest a historic window; rematch = re-run unmatched
    -- plays against the graph.
    run_type VARCHAR(20) NOT NULL
        CHECK (run_type IN ('discover', 'fetch', 'backfill', 'rematch')),
    -- Why the run happened. 'trigger' is a reserved SQL keyword, so the column is
    -- trigger_source (the Go field stays Trigger). scheduled = ticker; manual =
    -- admin "Sync now" / backfill; auto_backfill = on first discovery of a show.
    trigger_source VARCHAR(20) NOT NULL
        CHECK (trigger_source IN ('scheduled', 'manual', 'auto_backfill')),
    -- Lifecycle: running (open) -> one terminal state. partial = completed but the
    -- anomaly guard / per-episode errors flagged it (e.g. far fewer plays than the
    -- station's trailing average — the "successful but empty" signal). skipped =
    -- the breaker was open. cancelled = an in-flight backfill was aborted by an
    -- admin (carried forward from radio_import_jobs).
    status VARCHAR(20) NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'success', 'partial', 'failed', 'skipped', 'cancelled')),
    -- Requested historic backfill window. NULL on a normal scheduled/fetch run;
    -- non-NULL marks an operator (or auto) backfill over an explicit range — what
    -- makes a historic re-ingestion both parameterizable and observable in the
    -- feed. Replaces radio_import_jobs.since/until.
    window_start TIMESTAMPTZ,
    window_end TIMESTAMPTZ,
    -- The run opens with started_at known (DEFAULT NOW()); finished_at is NULL
    -- until a terminal status is reached.
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    -- Per-run counts. All default 0 so a freshly opened 'running' row is well-formed.
    episodes_found INTEGER NOT NULL DEFAULT 0,
    episodes_imported INTEGER NOT NULL DEFAULT 0,
    plays_imported INTEGER NOT NULL DEFAULT 0,
    plays_matched INTEGER NOT NULL DEFAULT 0,
    plays_unmatched INTEGER NOT NULL DEFAULT 0,
    plays_dropped INTEGER NOT NULL DEFAULT 0,
    plays_truncated INTEGER NOT NULL DEFAULT 0,
    -- True when the run returned early because the persistent circuit breaker was
    -- open for this station (pairs with status 'skipped').
    breaker_skipped BOOLEAN NOT NULL DEFAULT FALSE,
    -- Progress marker for the async backfill UI to poll (YYYY-MM-DD). Carried
    -- forward from radio_import_jobs.current_episode_date.
    current_episode_date VARCHAR(10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Integrity: a terminal time can't precede the start, and a window must be
    -- ordered (either bound may be NULL = unbounded/unknown). Mirrors the P1
    -- radio_episodes_air_window_check style.
    CONSTRAINT radio_sync_runs_finished_after_started_check
        CHECK (finished_at IS NULL OR finished_at >= started_at),
    CONSTRAINT radio_sync_runs_window_order_check
        CHECK (window_start IS NULL OR window_end IS NULL OR window_end >= window_start)
);

-- Newest-first per station (the per-station run feed, P5).
CREATE INDEX idx_radio_sync_runs_station_started
    ON radio_sync_runs (station_id, started_at DESC);
-- Newest-first global (the cross-station recent-failures feed, P5).
CREATE INDEX idx_radio_sync_runs_started
    ON radio_sync_runs (started_at DESC);
-- Status filter (e.g. "show me failed/partial runs").
CREATE INDEX idx_radio_sync_runs_status
    ON radio_sync_runs (status);

-- ============================================================================
-- radio_sync_run_errors — structured, queryable per-run errors.
-- ============================================================================
-- Child of radio_sync_runs. Generalizes PSY-1119's per-episode error capture to
-- every ingestion path and makes failures filterable by category instead of
-- grep-able only as free text in an error_log blob. Chosen over a JSONB column on
-- the run (open question in the ticket) precisely so categories are indexable.
CREATE TABLE radio_sync_run_errors (
    id SERIAL PRIMARY KEY,
    sync_run_id INTEGER NOT NULL REFERENCES radio_sync_runs(id) ON DELETE CASCADE,
    category VARCHAR(30) NOT NULL
        CHECK (category IN (
            'provider_unreachable', 'rate_limited', 'parse_error',
            'empty_unexpected', 'validation_drop', 'truncation',
            'match_persist_error', 'timeout'
        )),
    -- Human/machine-readable detail (provider message, validation reason, ...).
    detail TEXT,
    -- A SOFT reference to the episode an error concerns (provider date or external
    -- id), deliberately NOT a FK: an ingestion error frequently concerns an
    -- episode that FAILED to be created, so a hard FK would make the very failures
    -- this table exists to record unrecordable.
    episode_ref VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fetch a run's errors.
CREATE INDEX idx_radio_sync_run_errors_sync_run
    ON radio_sync_run_errors (sync_run_id);
-- Filter failures by category across runs.
CREATE INDEX idx_radio_sync_run_errors_category
    ON radio_sync_run_errors (category);

-- ============================================================================
-- radio_station_health — derived operational state, isolated from the entity.
-- ============================================================================
-- One row per station. Code Complete: isolate the volatile operational state from
-- the durable radio_stations entity so the entity stays clean and the breaker
-- survives restarts (today the breaker is in-memory and resets on every deploy —
-- a tripped station immediately retries after a restart). One-to-one with the
-- station; cascades on station delete.
CREATE TABLE radio_station_health (
    station_id INTEGER PRIMARY KEY REFERENCES radio_stations(id) ON DELETE CASCADE,
    last_success_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    -- Persistent circuit-breaker state (PSY-887 hardening, P3). closed = healthy;
    -- open = tripped, skip fetches; half_open = one trial fetch allowed.
    breaker_state VARCHAR(20) NOT NULL DEFAULT 'closed'
        CHECK (breaker_state IN ('closed', 'open', 'half_open')),
    breaker_tripped_at TIMESTAMPTZ,
    -- Rolled-up rates (0..1). NULLABLE on purpose: NULL = never computed, which is
    -- meaningfully different from 0.0 = computed and genuinely zero. The compute
    -- cadence (nightly vs on-write) is finalized with the P2 write path.
    recent_success_rate DOUBLE PRECISION,
    play_match_rate DOUBLE PRECISION,
    zero_play_episode_rate DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
