-- PSY-1291: "sync memo" for the ongoing artist-DISCOGRAPHY sweep (Phase A).
--
-- The sweep runs the MBID-keyed discography importer (discography.BackfillArtistDiscography:
-- MusicBrainz release-group browse + Cover Art Archive) on a slow cadence. Without a per-artist
-- sync timestamp the whole MBID-bearing catalog would be re-browsed every cycle — re-hitting
-- MusicBrainz at ~1 req/s per artist. This column lets the sweep skip rows synced within a
-- re-attempt window so a bounded nightly batch converges instead of re-hammering the providers.
-- Mirrors location_enrich_attempted_at (PSY-1250); kept a SEPARATE column because the two sweeps
-- converge independently.
ALTER TABLE artists ADD COLUMN discography_synced_at TIMESTAMPTZ;

-- Partial index matching the sweep's selection (MBID-bearing rows, ordered by sync time
-- NULLS FIRST then id) so a tick is an index scan rather than a full-table scan + sort each
-- cycle. Predicate matches the candidate gate exactly (TRIM, btrim is immutable).
CREATE INDEX idx_artists_discography_sync_pending
    ON artists (discography_synced_at NULLS FIRST, id)
    WHERE musicbrainz_artist_id IS NOT NULL AND TRIM(musicbrainz_artist_id) <> '';
