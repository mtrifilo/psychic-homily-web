-- PSY-1897 rollback.
--
-- Dropping artist_release_alert_batch destroys the release lists behind every
-- weekly roundup already delivered: the inbox rows survive but render with no
-- records named, and nothing reconstructs the membership. Re-applying the up
-- migration brings back an EMPTY table, not the old rows.
--
-- Dropping the unique index re-opens duplicate roundups for any week whose flush
-- runs while the index is absent, and the window is wide: the flush poller
-- re-resolves an already-delivered week whenever a late release joins it, and
-- the index is the only thing making that re-run silent. Stop the poller
-- (DISABLE_ARTIST_RELEASE_ALERTS=1) before rolling back if the process will keep
-- running.
--
-- Rows of entity_type 'artist_release_digest' are deliberately LEFT IN PLACE,
-- matching PSY-1895 and PSY-1896: they are real notifications real users
-- received, and deleting them would erase inbox history.
--
-- alert_bucket is NOT dropped here. That column belongs to
-- 20260824030000_venue_show_alert_batches, which is still in force below this
-- migration; dropping it would take the venue alert's uniqueness key with it and
-- leave the loop that owns it re-sending on every flush. The retained digest
-- rows therefore keep their buckets across the round trip, which is why the up
-- migration's backfill finds nothing to repair on the ordinary path — it is
-- there for the case where the venue migration was rolled back too.
--
-- The CHECK is dropped before the index, and both are named rather than left to
-- a cascade, so the rollback's blast radius is visible in the file.

ALTER TABLE notification_log
    DROP CONSTRAINT IF EXISTS ck_notification_log_release_digest_bucket;

DROP INDEX IF EXISTS uq_notification_log_artist_release_digest;

DROP TABLE IF EXISTS artist_release_alert_batch;
