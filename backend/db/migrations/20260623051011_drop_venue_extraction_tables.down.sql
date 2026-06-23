-- Reverse PSY-1165: recreate the venue-extraction tables at their final
-- pre-removal schema.
CREATE TABLE venue_source_configs (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    calendar_url TEXT,
    preferred_source VARCHAR(20) NOT NULL DEFAULT 'ai',
    render_method VARCHAR(20),
    feed_url TEXT,
    last_content_hash VARCHAR(64),
    last_etag TEXT,
    last_extracted_at TIMESTAMPTZ,
    events_expected INT NOT NULL DEFAULT 0,
    consecutive_failures INT NOT NULL DEFAULT 0,
    strategy_locked BOOLEAN NOT NULL DEFAULT false,
    auto_approve BOOLEAN NOT NULL DEFAULT false,
    extraction_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(venue_id)
);
CREATE INDEX idx_venue_source_configs_venue_id ON venue_source_configs(venue_id);

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
