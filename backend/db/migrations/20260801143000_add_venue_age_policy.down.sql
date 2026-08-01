-- PSY-1682 rollback. IF EXISTS keeps the down migration idempotent when it is
-- replayed against a database that never received the column.
ALTER TABLE venues
    DROP COLUMN IF EXISTS age_policy;
