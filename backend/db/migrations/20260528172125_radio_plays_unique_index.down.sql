-- PSY-888: Drop the radio_plays dedup unique index.

DROP INDEX CONCURRENTLY IF EXISTS idx_radio_plays_unique;
