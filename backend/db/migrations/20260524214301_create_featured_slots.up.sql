-- /explore landing has two admin-curated editorial slots — Featured Bill
-- (a show) and Featured Collection — with no automated rotation. Admins
-- curate on their own cadence; the currently active row stays visible
-- until it is retired or replaced.
--
-- One active row per slot_type at a time, enforced by a partial unique
-- index on (slot_type) WHERE active_until IS NULL. Retired rows keep the
-- history so we can show "previously featured" surfaces later without
-- mining the audit log.
--
-- entity_id is intentionally NOT a hard FK — the referent table varies
-- by slot_type ('bill' → shows, 'collection' → collections), and the
-- application layer (service.GetActiveSlot) resolves the right table.
-- This mirrors the polymorphic entity_id pattern already used by
-- pending_entity_edits and comments.

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

-- One currently-active row per slot_type. Retired rows (active_until
-- IS NOT NULL) are excluded so history stacks freely.
CREATE UNIQUE INDEX idx_featured_slots_active_per_type
    ON featured_slots (slot_type)
    WHERE active_until IS NULL;

-- History lookups (admin "recent picks" listing) hit (slot_type, created_at).
CREATE INDEX idx_featured_slots_slot_type_created_at
    ON featured_slots (slot_type, created_at DESC);
