-- Drop indexes
DROP INDEX IF EXISTS idx_artists_name_lower_prefix;
DROP INDEX IF EXISTS idx_artists_name_prefix;
DROP INDEX IF EXISTS idx_artists_name_trgm;

-- Note: We don't drop the extension as other tables might use it
-- DROP EXTENSION IF EXISTS pg_trgm;
