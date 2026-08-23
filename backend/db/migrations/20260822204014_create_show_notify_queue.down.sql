-- PSY-1894: drop the show-notification outbox.
--
-- Dropping the table discards the "already considered for notification" record
-- for every show in it. Re-applying the up migration afterwards therefore starts
-- from an empty table again, which is safe in the no-backfill direction (no
-- pre-existing show can be notified) but does mean a show notified before the
-- down could be re-notified if it were somehow re-enqueued after the re-up.
-- In practice re-enqueue only happens on a visibility transition, and the
-- per-(user, show) dedup in notification_log is the second line of defence.

DROP TABLE IF EXISTS show_notify_queue;
