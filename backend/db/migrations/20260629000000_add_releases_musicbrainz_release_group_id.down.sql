-- PSY-1281 down: drop the partial-unique index, then the column. (DROP COLUMN would
-- drop the dependent index implicitly, but be explicit and order-safe.)
DROP INDEX IF EXISTS uq_releases_musicbrainz_release_group_id;
ALTER TABLE releases DROP COLUMN IF EXISTS musicbrainz_release_group_id;
