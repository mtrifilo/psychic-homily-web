-- Carry a gated show's revision suppression across a show merge.
--
-- Shows are gated at the ENTITY level, not the field level: GET /shows/{id}
-- 404s a caller who is neither an admin nor the submitter when the show's
-- status is pending, rejected or private. Revision history now mirrors that
-- rule, and it reads shows.status for the show a revision currently points at.
--
-- catalog.MergeDuplicateShow breaks that read: it re-points a losing show's
-- revisions onto the winner and then DELETES the loser, so the row the gate
-- would have consulted is gone and a private show's history would resurface
-- under an approved winner. The dedup CLI selects losers from status IN
-- ('approved','private'), so this is a reachable path, not a hypothetical one.
-- The merge therefore stamps the loser's rows before re-pointing them, and the
-- read gate suppresses a stamped row whatever the show it now points at says.
--
-- This is the show-side twin of from_unverified_venue, and it is a SEPARATE
-- column rather than a shared "redacted" flag: the two carry different facts
-- (an address family withheld, versus a whole entity suppressed), the gates
-- that read them are keyed to different entity types, and merging them would
-- let a venue merge silently suppress show history or the reverse.
--
-- NOT NULL DEFAULT FALSE rather than nullable, for the same reasons as
-- from_unverified_venue: FALSE is the honest value for every existing row (no
-- merge has ever stamped one) and a two-valued column keeps the read gate a
-- plain boolean. Postgres 11+ fills the default from the catalog, so this does
-- not rewrite the table.
--
-- Deliberately NOT backfilled. Whether an already-merged show was gated at
-- merge time is not answerable from the schema, because the losing row is
-- deleted by the merge. The dedup runs are on record in audit_logs, so a
-- targeted backfill stays possible if it is ever wanted.
ALTER TABLE revisions
    ADD COLUMN IF NOT EXISTS from_gated_show BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN revisions.from_gated_show IS
    'True when this revision was re-pointed off a NON-APPROVED show by a show merge. The read-time visibility gate suppresses the row for non-admin callers regardless of the current show''s status.';
