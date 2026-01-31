-- Remove slug columns from artists, venues, and shows tables

DROP INDEX IF EXISTS idx_artists_slug;
DROP INDEX IF EXISTS idx_venues_slug;
DROP INDEX IF EXISTS idx_shows_slug;

ALTER TABLE artists DROP COLUMN IF EXISTS slug;
ALTER TABLE venues DROP COLUMN IF EXISTS slug;
ALTER TABLE shows DROP COLUMN IF EXISTS slug;
