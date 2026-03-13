CREATE TABLE revisions (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    field_changes JSONB NOT NULL,
    summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_revisions_entity ON revisions(entity_type, entity_id);
CREATE INDEX idx_revisions_user_id ON revisions(user_id);
CREATE INDEX idx_revisions_created_at ON revisions(created_at DESC);

-- Composite index for browsing entity history
CREATE INDEX idx_revisions_entity_created ON revisions(entity_type, entity_id, created_at DESC);
