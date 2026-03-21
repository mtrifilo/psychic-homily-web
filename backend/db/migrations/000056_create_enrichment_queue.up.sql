-- Create enrichment queue table for async post-import enrichment processing
CREATE TABLE enrichment_queue (
    id BIGSERIAL PRIMARY KEY,
    show_id BIGINT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT,
    enrichment_type VARCHAR(50) NOT NULL,
    results JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- Index on (status, created_at) for efficient queue polling
CREATE INDEX idx_enrichment_queue_status_created ON enrichment_queue (status, created_at);

-- Index on show_id for lookups
CREATE INDEX idx_enrichment_queue_show_id ON enrichment_queue (show_id);

COMMENT ON TABLE enrichment_queue IS 'Async enrichment queue for post-import processing (artist matching, MusicBrainz lookup, API cross-referencing)';
