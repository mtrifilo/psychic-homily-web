DROP INDEX IF EXISTS idx_artists_image_enrich_pending;
DROP INDEX IF EXISTS idx_releases_cover_enrich_pending;
ALTER TABLE artists  DROP COLUMN IF EXISTS image_enrich_attempted_at;
ALTER TABLE releases DROP COLUMN IF EXISTS image_enrich_attempted_at;
