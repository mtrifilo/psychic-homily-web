-- PSY-618: Backfill entity_edit_audit_logs from audit_logs and remove the
-- migrated rows. The trusted-user direct-edit dual-render (audit_logs +
-- pending_entity_edits in the contributor activity feed UNION) is fixed
-- structurally once these rows leave audit_logs. Stats counters are
-- updated separately to read the new table.
--
-- Source rows match action LIKE 'edit_%'. The trailing word after the
-- underscore is the entity type ("edit_artist" → "artist"). Migration
-- preserves actor_id, entity_id, metadata, and created_at so historical
-- stats and ordering are unchanged.

INSERT INTO entity_edit_audit_logs (actor_id, entity_type, entity_id, metadata, created_at)
SELECT
    actor_id,
    -- Strip "edit_" prefix from the action; the resulting value matches
    -- the entity_type column on audit_logs for these rows (e.g. "artist").
    SUBSTRING(action FROM 6) AS entity_type,
    entity_id,
    metadata,
    created_at
FROM audit_logs
WHERE action LIKE 'edit_%';

DELETE FROM audit_logs WHERE action LIKE 'edit_%';
