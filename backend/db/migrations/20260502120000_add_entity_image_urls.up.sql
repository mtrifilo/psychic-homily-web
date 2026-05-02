-- PSY-521: Add nullable image_url columns to artists, venues, shows, and labels.
--
-- These four entity types are surfaced in the PSY-360 collection visual grid
-- (and on their respective detail pages) but had no canonical image column —
-- the grid fell back to a typed Lucide icon for any item that wasn't a
-- release (cover_art_url) or festival (flyer_url).
--
-- VARCHAR(2048) matches the upper bound of common image-host URLs (S3, CDN
-- query strings) without the storage cost of TEXT. Nullable because curators
-- opt in per-record; no backfill ships with this migration. Image upload,
-- moderation, and external-source backfill are explicitly out of scope.
ALTER TABLE artists ADD COLUMN image_url VARCHAR(2048);
ALTER TABLE venues ADD COLUMN image_url VARCHAR(2048);
ALTER TABLE shows ADD COLUMN image_url VARCHAR(2048);
ALTER TABLE labels ADD COLUMN image_url VARCHAR(2048);
