-- Reverse the backfill: copy edit rows back into audit_logs, then truncate
-- the new table. action is reconstructed as 'edit_<entity_type>'. Note we
-- TRUNCATE rather than DROP so the schema-only down migration
-- (20260507043954_create_entity_edit_audit_logs.down.sql) handles table
-- removal.

INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, metadata, created_at)
SELECT
    actor_id,
    'edit_' || entity_type AS action,
    entity_type,
    entity_id,
    metadata,
    created_at
FROM entity_edit_audit_logs;

TRUNCATE TABLE entity_edit_audit_logs;
