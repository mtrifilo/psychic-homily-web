-- PSY-1131: Reverse the radio entity schema redesign. Mirror image of the up
-- migration. Restores the old air_timestamp-based radio_plays dedup index so
-- that, after this down runs, the prior migration's view of the schema is
-- intact (golang-migrate runs each down independently; the up→down→up CI
-- round-trip and a `down -all` both rely on each step being self-consistent).

-- ---------------------------------------------------------------------------
-- radio_plays
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_radio_plays_match_state;
DROP INDEX IF EXISTS idx_radio_plays_dedup;

-- Restore the dedup index this migration replaced. The original was created by
-- 20260528172125 with CREATE INDEX CONCURRENTLY (it was the only statement in
-- that single-statement file). This down file is multi-statement and therefore
-- runs inside a transaction, where CONCURRENTLY is illegal — so restore it as a
-- plain CREATE INDEX. Functionally identical; only the build strategy differs,
-- and the table is empty in the round-trip scenario.
CREATE UNIQUE INDEX IF NOT EXISTS idx_radio_plays_unique
    ON radio_plays (episode_id, position, air_timestamp, artist_name, track_title)
    NULLS NOT DISTINCT;

-- dedup_key is generated from provider_play_id; drop it before its source column.
ALTER TABLE radio_plays
    DROP COLUMN IF EXISTS dedup_key,
    DROP COLUMN IF EXISTS provider_play_id;

ALTER TABLE radio_plays
    DROP CONSTRAINT IF EXISTS radio_plays_rotation_status_check,
    DROP CONSTRAINT IF EXISTS radio_plays_match_state_check,
    DROP COLUMN IF EXISTS match_state;

-- ---------------------------------------------------------------------------
-- radio_episodes
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_radio_episodes_air_window;
DROP INDEX IF EXISTS idx_radio_episodes_status;

ALTER TABLE radio_episodes
    DROP CONSTRAINT IF EXISTS radio_episodes_air_window_check,
    DROP CONSTRAINT IF EXISTS radio_episodes_playlist_state_check,
    DROP CONSTRAINT IF EXISTS radio_episodes_status_check,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS playlist_fetched_at,
    DROP COLUMN IF EXISTS playlist_state,
    DROP COLUMN IF EXISTS ends_at,
    DROP COLUMN IF EXISTS starts_at,
    DROP COLUMN IF EXISTS status;

-- ---------------------------------------------------------------------------
-- radio_shows
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_radio_shows_station_name_lower;

ALTER TABLE radio_shows
    DROP CONSTRAINT IF EXISTS radio_shows_lifecycle_state_check,
    DROP CONSTRAINT IF EXISTS radio_shows_source_check,
    DROP COLUMN IF EXISTS lifecycle_state,
    DROP COLUMN IF EXISTS source;

-- ---------------------------------------------------------------------------
-- radio_stations
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_radio_stations_name_lower;

ALTER TABLE radio_stations
    DROP CONSTRAINT IF EXISTS radio_stations_playlist_source_check,
    DROP CONSTRAINT IF EXISTS radio_stations_broadcast_type_check,
    DROP CONSTRAINT IF EXISTS radio_stations_lifecycle_state_check,
    DROP CONSTRAINT IF EXISTS radio_stations_source_check,
    DROP COLUMN IF EXISTS lifecycle_state,
    DROP COLUMN IF EXISTS source;
