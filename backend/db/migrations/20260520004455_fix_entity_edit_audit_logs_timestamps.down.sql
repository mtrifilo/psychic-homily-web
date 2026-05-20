-- Revert PSY-754: restore the as-created column types from
-- 20260507043954_create_entity_edit_audit_logs.up.sql so the up->down->up
-- round-trip lands back on the original schema.
--
-- TIMESTAMPTZ -> TIMESTAMP records the wall-clock value in UTC (the session
-- runs in UTC), which is the inverse of the up migration's
-- `AT TIME ZONE 'UTC'` reinterpretation.

ALTER TABLE entity_edit_audit_logs
    ALTER COLUMN entity_id TYPE INT;

ALTER TABLE entity_edit_audit_logs
    ALTER COLUMN actor_id TYPE INT;

ALTER TABLE entity_edit_audit_logs
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';
