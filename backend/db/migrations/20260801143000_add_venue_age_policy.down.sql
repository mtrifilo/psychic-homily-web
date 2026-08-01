-- PSY-1682 rollback. IF EXISTS keeps the down migration idempotent when it is
-- replayed against a database that never received the column.
--
-- OPERATOR NOTE: roll the backend image back FIRST. GORM enumerates every model
-- column on Create, so running this while a build carrying Venue.AgePolicy is
-- live breaks every venue INSERT, including the public show-submission path
-- (FindOrCreateVenue), with: column "age_policy" does not exist.
--
-- This drops curated data. Values are human-contributed, not derivable, so the
-- only recovery is replaying revisions.field_changes / pending_entity_edits.
ALTER TABLE venues
    DROP COLUMN IF EXISTS age_policy;
