-- Dropping artist_release_alert_batch destroys the release lists behind every
-- delivered weekly roundup: the inbox rows survive but name no records, and
-- re-applying the up migration brings back an EMPTY table.
--
-- Dropping the unique index re-opens duplicate roundups, since the flush
-- re-resolves a delivered week whenever a late release joins it. Stop the
-- poller (DISABLE_ARTIST_RELEASE_ALERTS=1) before rolling back if the process
-- keeps running.
--
-- Rows of entity_type 'artist_release_digest' are LEFT IN PLACE: they are real
-- notifications users received.
--
-- alert_bucket is NOT dropped: it belongs to
-- 20260824030000_venue_show_alert_batches, whose uniqueness key depends on it.
-- The up migration's backfill covers the case where that migration was rolled
-- back too.

SET LOCAL lock_timeout = '5s';

ALTER TABLE notification_log
    DROP CONSTRAINT IF EXISTS ck_notification_log_release_digest_bucket;

DROP INDEX IF EXISTS uq_notification_log_artist_release_digest;

DROP TABLE IF EXISTS artist_release_alert_batch;
