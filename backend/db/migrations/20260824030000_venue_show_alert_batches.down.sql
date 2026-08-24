-- PSY-1895 rollback.
--
-- Dropping venue_show_alert_batch destroys the show lists behind every venue
-- alert already delivered: the inbox rows survive but render with no shows
-- named, and nothing reconstructs the membership. Re-applying the up migration
-- brings back an EMPTY table, not the old rows.
--
-- Dropping the unique index re-opens duplicate venue alerts for any batch whose
-- flush runs while the index is absent, and that is a wider window than the
-- artist equivalent: the flush poller re-resolves an already-delivered batch
-- whenever a late show joins it, and the index is the only thing making that
-- re-run silent. Stop the poller (DISABLE_VENUE_SHOW_ALERTS=1) before rolling
-- back if the process will keep running.
--
-- Rows of entity_type 'venue_show_alert' are deliberately LEFT IN PLACE, for the
-- same reason PSY-1896 leaves its own: they are real notifications real users
-- received, and deleting them would erase inbox history.
--
-- Note what those retained rows do NOT do: they provide no dedup on a re-up.
-- This file drops alert_bucket, so they come back carrying NULL, and NULLs
-- compare DISTINCT inside the re-created partial UNIQUE — they would match
-- nothing. What actually stops a second alert on a re-up is that
-- venue_show_alert_batch is dropped below and comes back EMPTY, so the flush
-- poller has nothing to resolve. (The up migration deletes those bucket-less
-- rows on the way back in, because they are unrenderable and would otherwise sit
-- outside the index forever.)
--
-- The CHECK is dropped before the column it constrains; the index before the
-- column it indexes. Postgres would cascade both, but naming them keeps the
-- rollback's blast radius visible in the file rather than implied.

ALTER TABLE notification_log
    DROP CONSTRAINT IF EXISTS ck_notification_log_venue_alert_bucket;

DROP INDEX IF EXISTS uq_notification_log_venue_show_alert;

ALTER TABLE notification_log
    DROP COLUMN IF EXISTS alert_bucket;

DROP TABLE IF EXISTS venue_show_alert_batch;
