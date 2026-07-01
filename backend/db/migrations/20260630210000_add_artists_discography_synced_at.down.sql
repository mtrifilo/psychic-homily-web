DROP INDEX IF EXISTS idx_artists_discography_sync_pending;
ALTER TABLE artists DROP COLUMN IF EXISTS discography_synced_at;
