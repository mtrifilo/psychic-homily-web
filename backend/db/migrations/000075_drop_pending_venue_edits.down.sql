-- PSY-503: Re-create pending_venue_edits table.
-- NOTE: This down migration restores the schema only. Any rows dropped by
-- the up migration are permanently lost — there is no undo for that data.

CREATE TYPE venue_edit_status AS ENUM ('pending', 'approved', 'rejected');

CREATE TABLE pending_venue_edits (
    id SERIAL PRIMARY KEY,
    venue_id INT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    submitted_by INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

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

    status venue_edit_status NOT NULL DEFAULT 'pending',
    rejection_reason TEXT,
    reviewed_by INT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pending_venue_edits_status ON pending_venue_edits(status);
CREATE INDEX idx_pending_venue_edits_venue_id ON pending_venue_edits(venue_id);
CREATE INDEX idx_pending_venue_edits_submitted_by ON pending_venue_edits(submitted_by);
