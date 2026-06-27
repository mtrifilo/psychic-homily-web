-- PSY-1246: "no-result memo" for the ongoing image-enrichment sweep. The sweep
-- runs the fill-when-empty enrichers (BackfillCommonsPhotos / BackfillCoverArt)
-- on a slow cadence; those key only on an empty image column, so without a
-- per-entity attempt timestamp the large imageless long tail (entities with no
-- MusicBrainz / Wikidata / Commons / CAA match) would be re-queried every cycle.
-- This column lets the sweep skip rows attempted within a re-attempt window so a
-- bounded nightly batch converges instead of re-hammering the providers.
ALTER TABLE artists  ADD COLUMN image_enrich_attempted_at TIMESTAMPTZ;
ALTER TABLE releases ADD COLUMN image_enrich_attempted_at TIMESTAMPTZ;

-- Partial indexes matching the sweep's selection (image-less rows, ordered by
-- attempt time NULLS FIRST then id), so a tick does an index scan rather than a
-- full table scan + sort of artists/releases each cycle.
CREATE INDEX idx_artists_image_enrich_pending
    ON artists (image_enrich_attempted_at NULLS FIRST, id)
    WHERE image_url IS NULL OR image_url = '';
CREATE INDEX idx_releases_cover_enrich_pending
    ON releases (image_enrich_attempted_at NULLS FIRST, id)
    WHERE cover_art_url IS NULL OR cover_art_url = '';
