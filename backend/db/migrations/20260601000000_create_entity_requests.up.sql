-- PSY-869: entity_requests — a polymorphic moderation queue for
-- user-requested ENTITY CREATION (not the existing `requests` wishlist).
--
-- Architectural decision (LOCKED 2026-05-26): a SINGLE polymorphic table,
-- NOT per-type request tables. Polymorphism lives in the table; typing
-- lives in Go (one payload struct per entity_type, validated on read via
-- UnmarshalPayload[T]). This foundation lets PSY-853 (AICollectionFiller)
-- and PSY-845 (AddItemsPicker) ship independently on top of it without
-- colliding on a migration.
--
-- Why this is NOT the existing `requests` table (migration 000049):
--   * `requests` is a community WISHLIST / voting surface — "I wish artist
--     X existed, vote it up" — with title/description/upvotes/fulfillment.
--   * `entity_requests` carries the DATA needed to CREATE an entity, queued
--     for admin moderation, with a typed JSONB payload per entity_type and
--     a trust-tier-gated auto-approve flow. Different lifecycle, different
--     payload, different consumers. They are deliberately separate tables.
--
-- Backfill note: the ticket asked to backfill an `artist_requests` table.
-- No such table exists in this schema (verified against db/migrations and
-- internal/models — the only request table is `requests`, which is the
-- wishlist feature above). Backfilling wishlist rows into this creation
-- queue would conflate two distinct features and seed the queue with rows
-- that have no creation payload, so this migration intentionally creates
-- the table EMPTY. See the PR body for the surfaced open question.
--
-- Polymorphic envelope mirrors pending_entity_edits (the canonical
-- polymorphic moderation table in this codebase): TEXT discriminator +
-- CHECK constraint for the enum, JSONB payload, nullable decision columns.

CREATE TABLE entity_requests (
    id BIGSERIAL PRIMARY KEY,

    -- Discriminator for the typed Go payload. CHECK-constrained rather than
    -- a Postgres ENUM type so adding a new entity_type is a one-line CHECK
    -- change in a follow-up migration (no ALTER TYPE ... ADD VALUE, which
    -- can't run in a transaction). Keep aligned with the Go payload registry
    -- in internal/models/community/entity_request_payloads.go — the CI
    -- parity check (scripts/check_entity_request_payloads.sh) fails the build
    -- if a value is added here without a matching payload struct.
    entity_type TEXT NOT NULL CHECK (
        entity_type IN ('artist', 'release', 'label', 'show', 'venue', 'festival')
    ),

    -- Type-specific fields, shape determined by entity_type and validated by
    -- the corresponding Go struct on read (UnmarshalPayload fails loud on
    -- schema drift / unknown fields). NOT NULL: a creation request with no
    -- payload is meaningless.
    payload JSONB NOT NULL,

    requester_id BIGINT NOT NULL REFERENCES users(id),

    -- How the request originated. CHECK-constrained enum, extensible the
    -- same way as entity_type.
    source_context TEXT NOT NULL DEFAULT 'manual' CHECK (
        source_context IN ('ai_extraction', 'paste_mode', 'manual')
    ),

    -- Moderation state. Trust-tier gating (service layer) decides whether a
    -- row lands 'pending' (queued for admin) or 'approved' (auto-approved
    -- for admin / trusted_contributor / local_ambassador).
    decision_state TEXT NOT NULL DEFAULT 'pending' CHECK (
        decision_state IN ('pending', 'approved', 'rejected')
    ),

    -- Who/when/why the decision was made. NULL while pending. decided_by is
    -- the admin (or the requester themselves on auto-approve) who resolved it.
    decided_by BIGINT REFERENCES users(id),
    decided_at TIMESTAMPTZ,
    decision_note TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Admin moderation queue scans by state (default view = pending) and often
-- filters by entity_type. The composite covers both the state-only and the
-- state+type access patterns.
CREATE INDEX idx_entity_requests_decision_state
    ON entity_requests (decision_state);
CREATE INDEX idx_entity_requests_state_type
    ON entity_requests (decision_state, entity_type);

-- "My requests" / requester-scoped lookups.
CREATE INDEX idx_entity_requests_requester_id
    ON entity_requests (requester_id);

-- Newest-first listing.
CREATE INDEX idx_entity_requests_created_at
    ON entity_requests (created_at DESC);
