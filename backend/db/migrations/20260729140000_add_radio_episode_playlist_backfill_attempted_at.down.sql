-- PSY-1562 rollback. The two data steps in the up migration are not reversed: the
-- memo seed dies with the column, and the scheduled-state repair restored rows to the
-- invariant the old code enforced on every re-list anyway, so re-corrupting them would
-- be strictly worse than leaving them correct.
DROP INDEX IF EXISTS idx_radio_episodes_backfill_candidates;

ALTER TABLE radio_episodes
    DROP COLUMN IF EXISTS playlist_backfill_attempted_at;
