CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_artists_local_lower
ON artists (LOWER(TRIM(city)), LOWER(TRIM(state)))
WHERE city IS NOT NULL AND city <> '' AND state IS NOT NULL AND state <> '';
