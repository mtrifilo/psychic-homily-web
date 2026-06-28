-- PSY-1250: "no-result memo" for the ongoing artist-LOCATION sweep (Phase A).
--
-- The sweep runs the fill-when-empty location resolver (enrich.BackfillArtistLocations:
-- MusicBrainz origin + Bandcamp self-report) on a slow cadence. That resolver keys
-- only on an empty city, so without a per-artist attempt timestamp the large
-- locationless long tail (artists with no MusicBrainz/Bandcamp match) would be
-- re-queried — re-hitting MusicBrainz at ~1 req/s — every cycle. This column lets the
-- sweep skip rows attempted within a re-attempt window so a bounded nightly batch
-- converges instead of re-hammering the providers. Mirrors image_enrich_attempted_at
-- (PSY-1246); kept a SEPARATE column because the two sweeps converge independently.
ALTER TABLE artists ADD COLUMN location_enrich_attempted_at TIMESTAMPTZ;

-- Partial index matching the sweep's selection (city-less rows, ordered by attempt
-- time NULLS FIRST then id) so a tick is an index scan rather than a full-table scan
-- + sort each cycle. Predicate matches the candidate gate exactly (TRIM, btrim is
-- immutable) so the planner can use it.
CREATE INDEX idx_artists_location_enrich_pending
    ON artists (location_enrich_attempted_at NULLS FIRST, id)
    WHERE city IS NULL OR TRIM(city) = '';
