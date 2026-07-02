-- PSY-1316 rollback: drop the sweep memo, provenance column, and the
-- backfill-scoped unique index.
DROP INDEX IF EXISTS uniq_release_links_backfill_per_platform;
ALTER TABLE release_external_links DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS idx_releases_links_enrich_pending;
ALTER TABLE releases DROP COLUMN IF EXISTS links_enrich_attempted_at;
