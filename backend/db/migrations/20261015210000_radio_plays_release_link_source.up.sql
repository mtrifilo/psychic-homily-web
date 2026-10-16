-- radio_plays.release_link_source names the writer that chose the play's
-- release_id: 'matcher' (a rule chose it: the matching engine, its bulk-link
-- passes, and the admin bulk-link endpoint, which picks no release itself),
-- 'admin' (the admin link-play endpoint), 'backfill' (the release-link
-- backfill). It is NULL when the play has no release link. A release deleted
-- out from under a play nulls release_id (ON DELETE SET NULL) and leaves this
-- column as it was, so it describes release_id only while release_id is set.
--
-- Existing links are marked here: 'admin' when a link_radio_play audit row
-- names the play and its current release, 'matcher' for every other link.
-- The audit read goes through idx_audit_logs_entity (entity_type, entity_id)
-- and the update touches only plays with a release link.
--
-- Adding the column takes an ACCESS EXCLUSIVE lock on radio_plays, held until
-- the marking UPDATE commits. lock_timeout bounds the wait for that lock, so a
-- long-running read fails this migration instead of queueing every later
-- query on the table behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE radio_plays
    ADD COLUMN release_link_source TEXT
        CONSTRAINT radio_plays_release_link_source_check
        CHECK (release_link_source IN ('matcher', 'admin', 'backfill'));

UPDATE radio_plays AS p
SET release_link_source = CASE
        WHEN EXISTS (
            SELECT 1
            FROM audit_logs AS al
            WHERE al.entity_type = 'radio_play'
              AND al.entity_id = p.id
              AND al.action = 'link_radio_play'
              AND al.metadata->>'release_id' = p.release_id::text
        ) THEN 'admin'
        ELSE 'matcher'
    END
WHERE p.release_id IS NOT NULL;
