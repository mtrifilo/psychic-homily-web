CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_shows_duplicate_of_show_id
ON shows (duplicate_of_show_id)
WHERE duplicate_of_show_id IS NOT NULL;
