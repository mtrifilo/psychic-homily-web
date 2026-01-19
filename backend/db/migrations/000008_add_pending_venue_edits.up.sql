-- Create pending venue edits table for non-admin users to propose venue changes
-- Similar to show approval workflow, venue edits from non-admins require admin approval

-- Create enum type for pending venue edit status
CREATE TYPE venue_edit_status AS ENUM ('pending', 'approved', 'rejected');

-- Create pending_venue_edits table
CREATE TABLE pending_venue_edits (
    id SERIAL PRIMARY KEY,
    venue_id INT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    submitted_by INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Proposed changes (null = no change to that field)
    name VARCHAR(255),
    address TEXT,
    city VARCHAR(100),
    state VARCHAR(50),
    zipcode VARCHAR(20),
    instagram VARCHAR(255),
    facebook VARCHAR(255),
    twitter VARCHAR(255),
    youtube VARCHAR(255),
    spotify VARCHAR(255),
    soundcloud VARCHAR(255),
    bandcamp VARCHAR(255),
    website VARCHAR(255),

    -- Workflow fields
    status venue_edit_status NOT NULL DEFAULT 'pending',
    rejection_reason TEXT,
    reviewed_by INT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMP,

    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for efficient filtering
CREATE INDEX idx_pending_venue_edits_status ON pending_venue_edits(status);
CREATE INDEX idx_pending_venue_edits_venue_id ON pending_venue_edits(venue_id);
CREATE INDEX idx_pending_venue_edits_submitted_by ON pending_venue_edits(submitted_by);

-- Add ownership tracking to venues (who originally submitted/created the venue)
ALTER TABLE venues ADD COLUMN submitted_by INT REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_venues_submitted_by ON venues(submitted_by);

-- Add comments for documentation
COMMENT ON TABLE pending_venue_edits IS 'Stores proposed venue edits from non-admin users awaiting admin approval';
COMMENT ON COLUMN pending_venue_edits.venue_id IS 'The venue being edited';
COMMENT ON COLUMN pending_venue_edits.submitted_by IS 'User who submitted the edit request';
COMMENT ON COLUMN pending_venue_edits.status IS 'Edit status: pending (awaiting review), approved (changes applied), rejected (discarded)';
COMMENT ON COLUMN pending_venue_edits.rejection_reason IS 'Admin-provided reason for rejecting the edit';
COMMENT ON COLUMN pending_venue_edits.reviewed_by IS 'Admin who reviewed the edit';
COMMENT ON COLUMN pending_venue_edits.reviewed_at IS 'When the edit was reviewed';
COMMENT ON COLUMN venues.submitted_by IS 'User ID of the person who originally submitted this venue';
