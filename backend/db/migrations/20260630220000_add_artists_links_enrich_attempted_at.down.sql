DROP INDEX IF EXISTS idx_artists_links_enrich_pending;
ALTER TABLE artists DROP COLUMN IF EXISTS links_enrich_attempted_at;
