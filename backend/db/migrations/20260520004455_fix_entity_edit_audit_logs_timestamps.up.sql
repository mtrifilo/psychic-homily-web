-- PSY-754: Fix entity_edit_audit_logs schema regression introduced when the
-- table was split out in PSY-618.
--
-- The table was created with `created_at TIMESTAMP` (no time zone) and 32-bit
-- `actor_id INT` / `entity_id INT`. Migration 000038 had already standardized
-- audit_logs.created_at to TIMESTAMPTZ; the new sibling table regressed that.
-- The writer (services/admin/audit_log.go) stores zoned UTC via
-- time.Now().UTC(), but a bare TIMESTAMP column drops the zone, which
-- mis-renders on read (the PSY-604/616 timezone bug class).
--
-- Converting TIMESTAMP -> TIMESTAMPTZ is a metadata-only change for an empty
-- or small table; `AT TIME ZONE 'UTC'` reinterprets existing naive values as
-- the UTC instants they were written as (matching the writer), rather than
-- letting PostgreSQL assume the server timezone.
--
-- Widening the foreign-key columns to BIGINT matches the BIGSERIAL primary key
-- and the Go model's `uint` fields (64-bit on our platforms), avoiding silent
-- failure past 2^31.

ALTER TABLE entity_edit_audit_logs
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

ALTER TABLE entity_edit_audit_logs
    ALTER COLUMN actor_id TYPE BIGINT;

ALTER TABLE entity_edit_audit_logs
    ALTER COLUMN entity_id TYPE BIGINT;
