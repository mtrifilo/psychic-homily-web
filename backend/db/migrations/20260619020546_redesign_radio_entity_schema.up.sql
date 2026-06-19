-- PSY-1131: Redesign radio entity schema — lifecycle, air-window, provenance,
-- constraints. Phase 1 (P1) of the greenfield Radio Ingestion Redesign.
--
-- This migration is GREENFIELD: it applies cleanly to a FRESH database (local
-- dev / CI seed). The strict constraints it adds (name uniqueness, NOT-NULL
-- enums) would reject dirty pre-existing rows on stage/prod, so the stage/prod
-- apply is deferred to the P6 cutover (wipe -> migrate -> re-ingest). Nothing
-- here is destructive to existing data; every change is additive (new columns
-- with backfilled DEFAULTs) or a constraint/index swap.
--
-- Enum representation decision: CHECK constraints (not Postgres enum TYPEs) for
-- ALL new enums. Rationale: during a greenfield rebuild every value set is still
-- volatile, and a CHECK is trivially reversible (DROP CONSTRAINT) whereas a
-- Postgres enum TYPE can't be dropped while any column still references it and
-- can't have values removed at all. Uniform CHECK keeps the down migration
-- simple and the value sets cheap to evolve. (Code Complete: isolate things
-- likely to change behind the cheapest-to-change mechanism.)

-- ============================================================================
-- radio_stations
-- ============================================================================

-- Provenance: how this station entered the graph. canonical = hand-curated
-- seed (KEXP/WFMU/NTS); discovered = created on first observed episode by the
-- ingestion pipeline (P2); manual = added by a human via admin UI.
ALTER TABLE radio_stations
    ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'canonical',
    ADD COLUMN lifecycle_state VARCHAR(20) NOT NULL DEFAULT 'active';

ALTER TABLE radio_stations
    ADD CONSTRAINT radio_stations_source_check
        CHECK (source IN ('canonical', 'discovered', 'manual')),
    ADD CONSTRAINT radio_stations_lifecycle_state_check
        CHECK (lifecycle_state IN ('active', 'dormant', 'retired'));

-- Enforce the previously app-only string enums at the DB boundary. broadcast_type
-- is NOT NULL (every station has one; default 'both'). playlist_source is
-- nullable — a NULL / link-only station has no automated provider — so the
-- CHECK permits NULL alongside the four provider tags.
ALTER TABLE radio_stations
    ADD CONSTRAINT radio_stations_broadcast_type_check
        CHECK (broadcast_type IN ('terrestrial', 'internet', 'both')),
    ADD CONSTRAINT radio_stations_playlist_source_check
        CHECK (playlist_source IS NULL
               OR playlist_source IN ('kexp_api', 'nts_api', 'wfmu_scrape', 'manual'));

-- Name uniqueness (case-insensitive). slug was already UNIQUE; the name was not,
-- so two stations could share a display name. Expression unique index on
-- lower(name) gives a case-insensitive guarantee.
CREATE UNIQUE INDEX idx_radio_stations_name_lower ON radio_stations (lower(name));

-- ============================================================================
-- radio_shows
-- ============================================================================

-- Provenance: provider = synced from a station's provider feed (KEXP/WFMU/NTS);
-- manual = added by a human. (No 'canonical' here — seeded shows are 'provider'
-- shows that happen to be pre-seeded; the distinction stations need between
-- canonical seed and pipeline-discovered does not apply to shows.)
ALTER TABLE radio_shows
    ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'provider',
    ADD COLUMN lifecycle_state VARCHAR(20) NOT NULL DEFAULT 'active';

ALTER TABLE radio_shows
    ADD CONSTRAINT radio_shows_source_check
        CHECK (source IN ('provider', 'manual')),
    ADD CONSTRAINT radio_shows_lifecycle_state_check
        CHECK (lifecycle_state IN ('active', 'dormant', 'retired'));

-- Name uniqueness scoped to the station (case-insensitive). Two stations may
-- legitimately each have a "Breakfast Show"; one station may not have two.
CREATE UNIQUE INDEX idx_radio_shows_station_name_lower
    ON radio_shows (station_id, lower(name));

-- The schedule JSONB is formalized as the validated shape
-- { "timezone": "America/Los_Angeles",
--   "slots": [ { "day_of_week": 1, "start": "06:00", "end": "10:00" } ] }
-- (day_of_week: 0=Sunday..6=Saturday; start/end: "HH:MM" 24h). This is the
-- basis for the air-window / "live" computation (consumed in P4). The Go side
-- validates the shape on write (catalog.RadioSchedule); the column itself stays
-- a plain JSONB so the validation lives in one place (the app boundary) rather
-- than being duplicated in a brittle JSONB CHECK. No schema change to the
-- existing `schedule JSONB` column is needed — only its contract is formalized.

-- ============================================================================
-- radio_episodes
-- ============================================================================

-- Episode lifecycle. status makes "live" an explicit, stored fact instead of
-- the implicit "a row exists for today's date" inference that produced the
-- false "ON AIR NOW" bug (PSY-1128). When no provider air window exists the
-- episode defaults to 'aired' and is NEVER 'live' (locked decision).
--   scheduled = announced, not yet aired (starts_at in the future)
--   live      = airing now (now between starts_at and ends_at)
--   aired     = finished airing (the default for windowless episodes)
--   archived  = aired + superseded / no longer the active airing
ALTER TABLE radio_episodes
    ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'aired',
    -- Real air window (timezone-aware). NULL when the provider gives no time —
    -- such an episode can never be computed 'live' (no window to be inside of).
    ADD COLUMN starts_at TIMESTAMPTZ,
    ADD COLUMN ends_at TIMESTAMPTZ,
    -- Playlist fetch lifecycle, decoupled from episode lifecycle.
    --   pending     = not yet fetched
    --   partial     = some plays fetched, fetch incomplete
    --   complete    = fully fetched
    --   unavailable = provider has no playlist for this episode
    ADD COLUMN playlist_state VARCHAR(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN playlist_fetched_at TIMESTAMPTZ,
    -- radio_episodes lacked an updated_at; add it so re-fetch / status
    -- transitions are observable. Default NOW() so existing rows get a sane
    -- value on a fresh-DB apply.
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE radio_episodes
    ADD CONSTRAINT radio_episodes_status_check
        CHECK (status IN ('scheduled', 'live', 'aired', 'archived')),
    ADD CONSTRAINT radio_episodes_playlist_state_check
        CHECK (playlist_state IN ('pending', 'partial', 'complete', 'unavailable')),
    -- A non-NULL air window must be ordered. Either-NULL is allowed (window
    -- unknown); both-non-NULL must satisfy ends_at >= starts_at.
    ADD CONSTRAINT radio_episodes_air_window_check
        CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at >= starts_at);

CREATE INDEX idx_radio_episodes_status ON radio_episodes (status);
-- Air-window lookups for the "live now" computation (P4): "episodes whose
-- window contains NOW()". Partial index skips the (majority, windowless) rows.
CREATE INDEX idx_radio_episodes_air_window
    ON radio_episodes (starts_at, ends_at)
    WHERE starts_at IS NOT NULL;

-- ============================================================================
-- radio_plays
-- ============================================================================

-- Matching lifecycle, replacing the implicit "artist_id IS NULL means unmatched"
-- with an explicit state the matching engine (P4) sets:
--   unmatched = not yet run through the matcher
--   matched   = resolved to exactly one knowledge-graph artist
--   ambiguous = matched multiple candidates, needs disambiguation
--   no_match  = matcher ran, found nothing (distinct from "not yet run")
ALTER TABLE radio_plays
    ADD COLUMN match_state VARCHAR(20) NOT NULL DEFAULT 'unmatched',
    -- Stable provider play id when the source supplies one (e.g. KEXP play id).
    -- NULL when the provider gives no stable id (NTS/WFMU) — the content-hash
    -- fallback in dedup_key covers those.
    ADD COLUMN provider_play_id VARCHAR(255);

ALTER TABLE radio_plays
    ADD CONSTRAINT radio_plays_match_state_check
        CHECK (match_state IN ('unmatched', 'matched', 'ambiguous', 'no_match'));

-- Enforce rotation_status at the DB boundary. NULL is allowed (most providers
-- don't supply a rotation; only KEXP does). The pipeline (P2/P4) is responsible
-- for normalizing an unrecognized provider rotation value to NULL BEFORE insert
-- — the CHECK enforces the stored vocabulary, it is not the normalization point.
ALTER TABLE radio_plays
    ADD CONSTRAINT radio_plays_rotation_status_check
        CHECK (rotation_status IS NULL
               OR rotation_status IN ('heavy', 'medium', 'light', 'recommended_new', 'library'));

-- Air-timestamp-independent dedup. The old idx_radio_plays_unique keyed on
-- air_timestamp, which NTS/WFMU don't always set, so re-imports of the same
-- playlist created duplicate rows. dedup_key is a GENERATED STORED column:
-- the stable provider play id when present, else an md5 content hash over
-- (position, artist_name, track_title, album_title). episode_id is NOT in the
-- hash because it's the other half of the unique key (a play id / content is
-- only unique WITHIN an episode). COALESCE the nullable text fields to '' so a
-- NULL track/album doesn't NULL out the whole hash. Being a generated column,
-- the value is computed by Postgres deterministically — no application code
-- needs to populate it, and identical re-imports produce identical keys.
--
-- NUL bytes can never reach this expression: Postgres text columns reject NUL
-- on insert outright, so artist_name/track_title/album_title are NUL-free by
-- the time the generated expression runs.
ALTER TABLE radio_plays
    ADD COLUMN dedup_key TEXT
        GENERATED ALWAYS AS (
            COALESCE(
                provider_play_id,
                md5(
                    position::text || '|' ||
                    artist_name || '|' ||
                    COALESCE(track_title, '') || '|' ||
                    COALESCE(album_title, '')
                )
            )
        ) STORED;

-- Drop the old air_timestamp-dependent dedup index and replace it with the
-- (episode_id, dedup_key) unique index. Both branches of dedup_key (provider id
-- and content hash) are non-NULL by construction, so no NULLS NOT DISTINCT is
-- required. The old index is created earlier in the migration history by
-- 20260528172125 (CONCURRENTLY, single-statement); we drop it plainly here
-- because this multi-statement migration runs in a transaction and the table
-- is empty on a fresh-DB apply.
DROP INDEX IF EXISTS idx_radio_plays_unique;
CREATE UNIQUE INDEX idx_radio_plays_dedup ON radio_plays (episode_id, dedup_key);

CREATE INDEX idx_radio_plays_match_state ON radio_plays (match_state);
