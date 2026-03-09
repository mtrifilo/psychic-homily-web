CREATE TABLE venue_extraction_runs (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    render_method VARCHAR(20),
    preferred_source VARCHAR(20),
    events_extracted INT NOT NULL DEFAULT 0,
    events_imported INT NOT NULL DEFAULT 0,
    content_hash VARCHAR(64),
    http_status INT,
    error TEXT,
    duration_ms INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_venue_extraction_runs_venue_id ON venue_extraction_runs(venue_id);
CREATE INDEX idx_venue_extraction_runs_run_at ON venue_extraction_runs(run_at);
