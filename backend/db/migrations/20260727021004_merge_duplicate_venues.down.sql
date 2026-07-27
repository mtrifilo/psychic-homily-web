-- Irreversible by design.
--
-- The up migration deletes duplicate show rows and repoints their references.
-- Once those rows are gone the original (loser_show_id -> winner_show_id)
-- mapping no longer exists anywhere in the database, so there is nothing to
-- reconstruct a rollback from. Re-splitting the merged venue would also mean
-- guessing which of the surviving shows had belonged to which record.
--
-- Faking a rollback here would be worse than admitting there isn't one: a
-- down migration that "succeeds" while restoring nothing turns a data-loss
-- event into a silent one. This file is intentionally a no-op so that
-- `migrate down` still walks past this version cleanly (the CI reversibility
-- job runs up -> down -all -> up on a fresh database and must not fail here).
--
-- To recover: restore from the pre-deploy database backup. The up migration
-- RAISEs a NOTICE with the venue-pair and duplicate-show counts, so the deploy
-- log records the size of what it removed.

SELECT 1;
