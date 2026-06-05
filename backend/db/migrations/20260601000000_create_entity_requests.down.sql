-- Reverse PSY-869. The table was created EMPTY (no backfill — see the .up.sql
-- header), so reversal is a clean DROP with no data to restore. Indexes drop
-- with the table.
DROP TABLE IF EXISTS entity_requests;
