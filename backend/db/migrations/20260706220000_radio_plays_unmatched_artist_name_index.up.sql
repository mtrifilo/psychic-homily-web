-- PSY-1366: partial index for chunked rematch distinct-name enumeration.
-- Sweeps filter match_state = 'unmatched'; this avoids scanning exhausted
-- no_match / ambiguous rows when paging artist_name.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_radio_plays_unmatched_artist_name
  ON radio_plays (artist_name)
  WHERE artist_id IS NULL AND match_state = 'unmatched';
