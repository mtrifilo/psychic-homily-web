-- Widen audit_logs.actor_id / entity_id from 32-bit INT to BIGINT.
--
-- Sibling fix to the entity_edit_audit_logs ID widening: audit_logs (created
-- in 000022) still carries `actor_id INT` / `entity_id INT`, even though its
-- `id` is BIGSERIAL and the Go model (models/admin/audit_log.go) types both
-- columns as `uint` (64-bit on our platforms). Migration 000038 standardized
-- only this table's created_at to TIMESTAMPTZ, not the ID widths, leaving a
-- latent 2^31 overflow once entity ids exceed that range.
--
-- This is a metadata-only widening; INT -> BIGINT preserves all values. The
-- actor_id FK to users(id) (still SERIAL/int4) stays valid because PostgreSQL
-- compares int4 and int8 across a foreign-key boundary without issue.

ALTER TABLE audit_logs
    ALTER COLUMN actor_id TYPE BIGINT;

ALTER TABLE audit_logs
    ALTER COLUMN entity_id TYPE BIGINT;
