-- Carry an unverified venue's address redaction across a venue merge.
-- Policy and mechanism: catalog.markUnverifiedVenueRevisions and the privacy
-- section of the revisiondiff package doc.
--
-- NOT NULL DEFAULT FALSE rather than nullable: FALSE is the honest value for
-- every existing row (no merge has ever stamped one), and a two-valued column
-- keeps the read gate a plain boolean rather than a three-way check. Postgres
-- 11+ fills the default from the catalog, so this does not rewrite the table
-- and leaves no dead tuples behind.
--
-- Deliberately NOT backfilled, and the unmarked population is BOUNDED rather
-- than open-ended. Whether an already-merged venue was unverified at merge time
-- is not answerable from the schema (the losing rows are gone), but the merges
-- themselves are on record: audit_logs where action = 'merge_venues', plus the
-- two pairs named by slug in 20260727021004_merge_duplicate_venues.up.sql
-- (7th St Entry -> 7th Street Entry, Metro Gallery -> Metro Baltimore, both
-- real public rooms). That is a hand-inspectable set, so a backfill is a
-- targeted UPDATE if it is ever wanted, not a migration-wide sweep.
ALTER TABLE revisions
    ADD COLUMN IF NOT EXISTS from_unverified_venue BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN revisions.from_unverified_venue IS
    'True when this revision was re-pointed off an UNVERIFIED venue by a venue merge. Read-time address redaction masks the row regardless of the current venue''s verified state.';
