-- Rollback. Nothing else references these columns, and event_date is
-- untouched by the up migration, so dropping them restores the prior schema
-- exactly. Any doors/music values entered while they existed are lost.
ALTER TABLE shows
    DROP COLUMN IF EXISTS music_at,
    DROP COLUMN IF EXISTS doors_at;
