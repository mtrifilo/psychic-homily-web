-- Reversing this is safe in the direction that matters: show_dedup_keys enforces
-- a strict superset of shows_artist_venue_eventdate_uniq, so any data written
-- while it was in force already satisfies the weaker index left behind, and the
-- table is derived, so dropping it loses nothing that cannot be rebuilt.
DROP TRIGGER IF EXISTS shows_sync_dedup_keys ON shows;
DROP TRIGGER IF EXISTS show_venues_sync_dedup_keys ON show_venues;
DROP TRIGGER IF EXISTS show_artists_sync_dedup_keys ON show_artists;

DROP FUNCTION IF EXISTS show_dedup_keys_sync_show();
DROP FUNCTION IF EXISTS show_dedup_keys_sync_link();
DROP FUNCTION IF EXISTS show_dedup_keys_rebuild(INT);

DROP TABLE IF EXISTS show_dedup_keys;
