DROP INDEX IF EXISTS idx_artists_streaming_discovery_status;

ALTER TABLE artists
    DROP COLUMN IF EXISTS streaming_discovery_reason,
    DROP COLUMN IF EXISTS streaming_discovery_status;
