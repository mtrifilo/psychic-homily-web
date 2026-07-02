DROP TABLE IF EXISTS artist_communities;

DROP INDEX IF EXISTS idx_artists_community;

ALTER TABLE artists DROP COLUMN IF EXISTS community_id;
