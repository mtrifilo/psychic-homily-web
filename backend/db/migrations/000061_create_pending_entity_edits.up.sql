-- Generic pending entity edits table.
-- Replaces the per-entity-type approach (pending_venue_edits) with a single
-- table using JSONB field_changes — same format as the revisions table.
CREATE TABLE pending_entity_edits (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    submitted_by BIGINT NOT NULL REFERENCES users(id),
    field_changes JSONB NOT NULL,
    summary TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewed_by BIGINT REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One pending edit per user per entity (only enforced for pending status)
CREATE UNIQUE INDEX idx_pending_entity_edits_unique
    ON pending_entity_edits (entity_type, entity_id, submitted_by)
    WHERE status = 'pending';

-- Fast lookup for the admin review queue (oldest pending first)
CREATE INDEX idx_pending_entity_edits_status
    ON pending_entity_edits (status, created_at);

-- Fast lookup for a user's own edits
CREATE INDEX idx_pending_entity_edits_user
    ON pending_entity_edits (submitted_by, status);

-- Fast lookup for edits on a specific entity
CREATE INDEX idx_pending_entity_edits_entity
    ON pending_entity_edits (entity_type, entity_id);
