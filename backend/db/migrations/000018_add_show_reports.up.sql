-- Create show reports table for users to report issues with shows
-- Report types: cancelled, sold_out, inaccurate
-- Status flow: pending -> dismissed (spam/invalid) or resolved (action taken)

-- Create enum type for show report type
CREATE TYPE show_report_type AS ENUM ('cancelled', 'sold_out', 'inaccurate');

-- Create enum type for show report status
CREATE TYPE show_report_status AS ENUM ('pending', 'dismissed', 'resolved');

-- Create show_reports table
CREATE TABLE show_reports (
    id SERIAL PRIMARY KEY,
    show_id INT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    reported_by INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    report_type show_report_type NOT NULL,
    details TEXT,
    status show_report_status NOT NULL DEFAULT 'pending',
    admin_notes TEXT,
    reviewed_by INT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(show_id, reported_by)  -- One report per user per show
);

-- Create indexes for efficient filtering
CREATE INDEX idx_show_reports_status ON show_reports(status);
CREATE INDEX idx_show_reports_show_id ON show_reports(show_id);
CREATE INDEX idx_show_reports_created_at ON show_reports(created_at DESC);

-- Add comments for documentation
COMMENT ON TABLE show_reports IS 'Stores user reports about show issues (cancelled, sold out, inaccurate info)';
COMMENT ON COLUMN show_reports.show_id IS 'The show being reported';
COMMENT ON COLUMN show_reports.reported_by IS 'User who submitted the report';
COMMENT ON COLUMN show_reports.report_type IS 'Type of issue: cancelled, sold_out, or inaccurate';
COMMENT ON COLUMN show_reports.details IS 'Optional details about the issue (primarily for inaccurate reports)';
COMMENT ON COLUMN show_reports.status IS 'Report status: pending (awaiting review), dismissed (spam/invalid), resolved (action taken)';
COMMENT ON COLUMN show_reports.admin_notes IS 'Admin notes about the resolution';
COMMENT ON COLUMN show_reports.reviewed_by IS 'Admin who reviewed the report';
COMMENT ON COLUMN show_reports.reviewed_at IS 'When the report was reviewed';
