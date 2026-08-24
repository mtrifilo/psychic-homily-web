-- PSY-1896 rollback.
--
-- Dropping the index re-opens duplicate artist show alerts for any (user, show,
-- channel) whose row is re-processed while the index is absent; dropping the
-- column blanks the followed-artist label on every alert already delivered.
-- Both are recoverable by re-applying the up migration, except that the labels
-- are gone for good: nothing reconstructs subject_entity_id.
--
-- Rows of entity_type 'artist_show_alert' are deliberately LEFT IN PLACE. They
-- are real notifications real users received, and deleting them would both erase
-- inbox history and, on a re-up, allow every one of those shows to alert its
-- followers a second time.

DROP INDEX IF EXISTS uq_notification_log_artist_show_alert;

ALTER TABLE notification_log
    DROP COLUMN IF EXISTS subject_entity_id;
