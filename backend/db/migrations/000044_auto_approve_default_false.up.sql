-- Change auto_approve default from true to false for new venue source configs.
-- Pipeline-imported shows should default to pending for admin review.
ALTER TABLE venue_source_configs ALTER COLUMN auto_approve SET DEFAULT false;

-- Update existing rows to false (no venues should auto-approve until explicitly enabled).
UPDATE venue_source_configs SET auto_approve = false WHERE auto_approve = true;
