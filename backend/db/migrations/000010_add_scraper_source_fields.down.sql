-- Rollback source tracking fields for scraped shows

-- Drop indexes
DROP INDEX IF EXISTS idx_shows_source_dedup;
DROP INDEX IF EXISTS idx_shows_source;

-- Drop columns
ALTER TABLE shows DROP COLUMN IF EXISTS scraped_at;
ALTER TABLE shows DROP COLUMN IF EXISTS source_event_id;
ALTER TABLE shows DROP COLUMN IF EXISTS source_venue;
ALTER TABLE shows DROP COLUMN IF EXISTS source;

-- Drop enum type
DROP TYPE IF EXISTS show_source;
