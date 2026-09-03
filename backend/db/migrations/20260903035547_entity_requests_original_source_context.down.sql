-- Reverse PSY-1978's original_source_context column.
--
-- DESTRUCTIVE, and in the one direction that cannot be reconstructed from the
-- table: the column is the only place a row states what it was originally filed
-- as, and a resubmission has already overwritten source_context with the current
-- claim.
--
-- The replace_entity_request audit_logs rows name the superseded source_context
-- per replacement, but they are NOT a guarantee: that write is fire-and-forget
-- after the response and its failures are logged, not retried. This column is
-- written in the same statement as the write it describes, so dropping it makes
-- the fact best-effort.
--
-- What the column records is the FIRST filing, so it is not a complete history
-- either: a row filed manual, revised to ai_extraction and revised back reads as
-- unchanged. The audit rows are what cover the intermediate states.
--
-- Roll the BINARY back first, then run this. A binary that writes this column
-- against a table without it fails every replacement with an undefined-column
-- error, which turns a contributor's correction into a 500 — the usual ordering
-- for a dropped column, and unlike the dedup-index migrations, which invert it.

ALTER TABLE entity_requests
    DROP COLUMN IF EXISTS original_source_context;
