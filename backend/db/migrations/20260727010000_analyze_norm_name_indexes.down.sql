-- PSY-1570 follow-up rollback: intentionally a no-op.
--
-- ANALYZE only refreshes planner statistics; there is no "un-analyze", and
-- discarding statistics would be a pessimisation, not a rollback. Rolling back
-- 20260726230000 drops the indexes, at which point their expression statistics
-- become irrelevant on their own.
--
-- A statement is still required here so the migration runner has something to
-- execute rather than an empty query.
SELECT 1;
