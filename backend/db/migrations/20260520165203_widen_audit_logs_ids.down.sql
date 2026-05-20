-- Revert the ID widening: restore the as-created INT column types from
-- 000022_add_audit_logs.up.sql so the up->down->up round-trip lands back on
-- the original schema. Narrowing BIGINT -> INT is safe here because the
-- forward migration only widened the type without introducing out-of-range
-- values.

ALTER TABLE audit_logs
    ALTER COLUMN entity_id TYPE INT;

ALTER TABLE audit_logs
    ALTER COLUMN actor_id TYPE INT;
