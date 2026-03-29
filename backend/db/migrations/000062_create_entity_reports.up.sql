-- Create the generic entity_reports table.
-- Replaces the need for per-entity report tables (show_reports, artist_reports, etc.)
-- by using entity_type + entity_id polymorphism — same pattern as pending_entity_edits.

CREATE TABLE entity_reports (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    reported_by BIGINT NOT NULL REFERENCES users(id),
    report_type VARCHAR(50) NOT NULL,
    details TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    admin_notes TEXT,
    reviewed_by BIGINT REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_entity_reports_entity ON entity_reports (entity_type, entity_id);
CREATE INDEX idx_entity_reports_status_created ON entity_reports (status, created_at);
CREATE INDEX idx_entity_reports_reported_by ON entity_reports (reported_by);
