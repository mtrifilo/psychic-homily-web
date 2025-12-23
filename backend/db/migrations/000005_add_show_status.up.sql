-- Add status tracking for show submissions
-- Shows with unverified venues require admin approval before being publicly visible

-- Create enum type for show status
CREATE TYPE show_status AS ENUM ('pending', 'approved', 'rejected');

-- Add status column to shows table (default 'approved' for existing shows)
ALTER TABLE shows ADD COLUMN status show_status NOT NULL DEFAULT 'approved';

-- Add submitted_by to track who submitted the show
ALTER TABLE shows ADD COLUMN submitted_by INT REFERENCES users(id) ON DELETE SET NULL;

-- Add rejection_reason for admin feedback when rejecting shows
ALTER TABLE shows ADD COLUMN rejection_reason TEXT;

-- Create indexes for efficient filtering
CREATE INDEX idx_shows_status ON shows(status);
CREATE INDEX idx_shows_submitted_by ON shows(submitted_by);

-- Add comments for documentation
COMMENT ON COLUMN shows.status IS 'Show approval status: pending (awaiting admin review), approved (visible to public), rejected (not shown)';
COMMENT ON COLUMN shows.submitted_by IS 'User ID of the person who submitted this show';
COMMENT ON COLUMN shows.rejection_reason IS 'Admin-provided reason for rejecting the show submission';
