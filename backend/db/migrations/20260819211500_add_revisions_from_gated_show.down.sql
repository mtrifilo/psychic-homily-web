-- Dropping this column DISCARDS the only surviving record that a merged-away
-- show was gated: the losing show row is deleted by the merge, so the marks
-- cannot be re-derived from the schema. Re-applying the up migration gives back
-- the column, not the marks. Down is for a failed deploy, not for a rollback of
-- a database that has since run the dedup CLI.
ALTER TABLE revisions
    DROP COLUMN IF EXISTS from_gated_show;
