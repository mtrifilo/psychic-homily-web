-- Reverse PSY-1978's original_source_context column.
--
-- DESTRUCTIVE, and in the one direction that cannot be reconstructed from the
-- table: the column is the only place a row states what it was originally filed
-- as, and a resubmission has already overwritten source_context with the current
-- claim. The replace_entity_request audit_logs rows still carry the superseded
-- values per replacement, so the fact survives the drop; only the moderation
-- card's ability to show it without an audit query does not.
--
-- Roll the BINARY back first, then run this. A binary that writes this column
-- against a table without it fails every replacement with an undefined-column
-- error, which turns a contributor's correction into a 500 — the usual ordering
-- for a dropped column, and unlike the dedup-index migrations, which invert it.

ALTER TABLE entity_requests
    DROP COLUMN IF EXISTS original_source_context;
