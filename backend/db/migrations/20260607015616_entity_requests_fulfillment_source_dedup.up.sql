-- PSY-1008: entity_requests — record fulfillment outcome, carry AI source
-- context, and dedup duplicate pending requests. Builds on PSY-869's table
-- (20260601000000_create_entity_requests) + PSY-997's HTTP endpoints.
--
-- Multi-statement file ⇒ golang-migrate v4 wraps it in a transaction, so the
-- index below is a plain CREATE (NOT CONCURRENTLY — that can't run in a txn;
-- see 20260528172125_radio_plays_unique_index for the single-statement
-- CONCURRENTLY case). entity_requests is a small, recently-created dogfood
-- table, so a transactional index build does not lock meaningful write traffic.

-- created_entity_id: the catalog entity row created when this request was
-- fulfilled (auto-approve create OR admin approve). It is a CROSS-TYPE id —
-- it points into one of several catalog tables (artists/venues/labels/...),
-- discriminated by entity_type — so there is intentionally NO foreign key.
-- NULL means: still pending, rejected, or an ORPHANED approval (approved but
-- fulfillment failed / was deferred, e.g. show/festival). The admin queue
-- (PSY-871) uses (decision_state = 'approved' AND created_entity_id IS NULL)
-- to surface approvals that still need a real entity created.
ALTER TABLE entity_requests
    ADD COLUMN created_entity_id BIGINT;

-- source_detail: optional structured context for the request's origin (chiefly
-- AI extraction): the source article URL + excerpt the requester saw, so the
-- admin moderation surface can show the requester's intent for AI-origin rows.
-- Typed in Go (community.EntityRequestSourceDetail) and stored opaquely as
-- JSONB, mirroring the `payload` column's polymorphism-in-table /
-- typing-in-Go convention. Distinct from source_context (the origin enum).
ALTER TABLE entity_requests
    ADD COLUMN source_detail JSONB;

-- Dedup: at most one PENDING request per (entity_type, requester, normalized
-- name). Partial (pending-only) so a user can legitimately re-request after a
-- prior request was approved or rejected. The name is the lower(trim(...)) of
-- the payload's name-or-title field — artist/venue/label/festival carry 'name',
-- release/show carry 'title' — extracted with the immutable jsonb ->> operator
-- so the expression is index-eligible. Auto-approved rows are never 'pending',
-- so this index does NOT constrain the auto-approve path (dedup there is the
-- frontend match-before-queue step + the catalog's own concern).
CREATE UNIQUE INDEX uq_entity_requests_pending_dedup
    ON entity_requests (
        entity_type,
        requester_id,
        (lower(trim(coalesce(payload->>'name', payload->>'title'))))
    )
    WHERE decision_state = 'pending';
