DROP INDEX IF EXISTS idx_artists_location_enrich_pending;
ALTER TABLE artists DROP COLUMN IF EXISTS location_enrich_attempted_at;
