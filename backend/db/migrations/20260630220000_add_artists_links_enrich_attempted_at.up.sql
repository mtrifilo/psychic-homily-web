-- PSY-1279: "no-result memo" for the ongoing artist-LINKS sweep (Phase A).
--
-- The sweep runs MBID-keyed MusicBrainz url-rel lookup (enrich.BackfillArtistLinks:
-- spotify/bandcamp/website fill-when-empty) on a slow cadence. Without a per-artist
-- attempt timestamp the link-partial long tail would be re-queried every cycle — re-hitting
-- MusicBrainz at ~1 req/s. This column lets the sweep skip rows attempted within a
-- re-attempt window so a bounded nightly batch converges. Mirrors
-- location_enrich_attempted_at (PSY-1250); kept SEPARATE because the two sweeps converge
-- independently.
ALTER TABLE artists ADD COLUMN links_enrich_attempted_at TIMESTAMPTZ;

-- Partial index matching the sweep's selection (MBID-bearing rows missing any target link,
-- ordered by attempt time NULLS FIRST then id).
CREATE INDEX idx_artists_links_enrich_pending
    ON artists (links_enrich_attempted_at NULLS FIRST, id)
    WHERE musicbrainz_artist_id IS NOT NULL AND TRIM(musicbrainz_artist_id) <> ''
      AND (
          spotify IS NULL OR TRIM(spotify) = '' OR
          bandcamp IS NULL OR TRIM(bandcamp) = '' OR
          website IS NULL OR TRIM(website) = ''
      );
