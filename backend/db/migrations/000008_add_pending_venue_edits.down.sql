-- Rollback pending venue edits feature

-- Remove submitted_by from venues
DROP INDEX IF EXISTS idx_venues_submitted_by;
ALTER TABLE venues DROP COLUMN IF EXISTS submitted_by;

-- Drop pending_venue_edits table
DROP TABLE IF EXISTS pending_venue_edits;

-- Drop venue_edit_status enum type
DROP TYPE IF EXISTS venue_edit_status;
