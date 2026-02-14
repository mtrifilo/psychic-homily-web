-- Create artist report type enum
CREATE TYPE artist_report_type AS ENUM ('inaccurate', 'removal_request');

-- Create artist_reports table (reuses existing show_report_status enum)
CREATE TABLE artist_reports (
    id SERIAL PRIMARY KEY,
    artist_id INTEGER NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    reported_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    report_type artist_report_type NOT NULL,
    details TEXT,
    status show_report_status NOT NULL DEFAULT 'pending',
    admin_notes TEXT,
    reviewed_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One report per user per artist
    UNIQUE(artist_id, reported_by)
);

-- Indexes for common queries
CREATE INDEX idx_artist_reports_status ON artist_reports(status);
CREATE INDEX idx_artist_reports_artist_id ON artist_reports(artist_id);
CREATE INDEX idx_artist_reports_created_at ON artist_reports(created_at DESC);
