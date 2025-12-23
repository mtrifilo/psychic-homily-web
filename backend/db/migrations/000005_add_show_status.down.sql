-- Revert show status tracking changes

-- Drop indexes
DROP INDEX IF EXISTS idx_shows_status;
DROP INDEX IF EXISTS idx_shows_submitted_by;

-- Remove columns from shows table
ALTER TABLE shows DROP COLUMN IF EXISTS rejection_reason;
ALTER TABLE shows DROP COLUMN IF EXISTS submitted_by;
ALTER TABLE shows DROP COLUMN IF EXISTS status;

-- Drop the enum type
DROP TYPE IF EXISTS show_status;
