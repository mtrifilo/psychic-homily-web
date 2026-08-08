-- Dropping this column DISCARDS the only surviving record that a merged-away
-- venue was unverified: the losing venue row is deleted by the merge, so the
-- marks cannot be re-derived from the schema. Re-applying the up migration
-- gives back the column, not the marks; recovering those means re-reading
-- audit_logs for merge_venues. Down is for a failed deploy, not for a rollback
-- of a database that has since served merges.
ALTER TABLE revisions
    DROP COLUMN IF EXISTS from_unverified_venue;
