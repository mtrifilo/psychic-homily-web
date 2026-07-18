-- Restore featured_slots schema (curated content is not recoverable).
CREATE TABLE featured_slots (
    id BIGSERIAL PRIMARY KEY,
    slot_type TEXT NOT NULL CHECK (slot_type IN ('bill', 'collection')),
    entity_id BIGINT NOT NULL,
    curator_note TEXT,
    active_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    active_until TIMESTAMPTZ,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_featured_slots_active_per_type
    ON featured_slots (slot_type)
    WHERE active_until IS NULL;

CREATE INDEX idx_featured_slots_slot_type_created_at
    ON featured_slots (slot_type, created_at DESC);
