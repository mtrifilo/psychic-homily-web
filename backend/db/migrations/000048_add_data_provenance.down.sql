-- Drop indexes first, then columns from all 6 tables

-- Shows
DROP INDEX IF EXISTS idx_shows_data_source;
DROP INDEX IF EXISTS idx_shows_last_verified_at;
ALTER TABLE shows
    DROP CONSTRAINT IF EXISTS chk_shows_source_confidence,
    DROP COLUMN IF EXISTS data_source,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS last_verified_at;

-- Artists
DROP INDEX IF EXISTS idx_artists_data_source;
DROP INDEX IF EXISTS idx_artists_last_verified_at;
ALTER TABLE artists
    DROP CONSTRAINT IF EXISTS chk_artists_source_confidence,
    DROP COLUMN IF EXISTS data_source,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS last_verified_at;

-- Venues
DROP INDEX IF EXISTS idx_venues_data_source;
DROP INDEX IF EXISTS idx_venues_last_verified_at;
ALTER TABLE venues
    DROP CONSTRAINT IF EXISTS chk_venues_source_confidence,
    DROP COLUMN IF EXISTS data_source,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS last_verified_at;

-- Releases
DROP INDEX IF EXISTS idx_releases_data_source;
DROP INDEX IF EXISTS idx_releases_last_verified_at;
ALTER TABLE releases
    DROP CONSTRAINT IF EXISTS chk_releases_source_confidence,
    DROP COLUMN IF EXISTS data_source,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS last_verified_at;

-- Labels
DROP INDEX IF EXISTS idx_labels_data_source;
DROP INDEX IF EXISTS idx_labels_last_verified_at;
ALTER TABLE labels
    DROP CONSTRAINT IF EXISTS chk_labels_source_confidence,
    DROP COLUMN IF EXISTS data_source,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS last_verified_at;

-- Festivals
DROP INDEX IF EXISTS idx_festivals_data_source;
DROP INDEX IF EXISTS idx_festivals_last_verified_at;
ALTER TABLE festivals
    DROP CONSTRAINT IF EXISTS chk_festivals_source_confidence,
    DROP COLUMN IF EXISTS data_source,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS last_verified_at;
