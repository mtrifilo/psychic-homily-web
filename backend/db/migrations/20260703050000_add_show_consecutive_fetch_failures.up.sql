-- PSY-1274: per-show sustained-outage escalation.
-- Counts consecutive provider fetch failures for a show's episode-listing call.
-- Incremented by the fetch loop when a show's provider fetch errors, reset to 0 on
-- the next successful fetch (a fetch that succeeds but returns zero episodes is a
-- SUCCESS — infrequent shows never accumulate a streak, keeping the signal
-- cadence-independent). The janitor escalates shows whose streak crosses the
-- threshold on an otherwise-healthy station (radio_sync.go).
ALTER TABLE radio_shows
    ADD COLUMN consecutive_fetch_failures INTEGER NOT NULL DEFAULT 0;
