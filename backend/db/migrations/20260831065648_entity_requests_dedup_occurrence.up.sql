-- PSY-1977: add the OCCURRENCE to the pending-request dedup key, so two
-- genuinely different requests that share a name stop colliding.
--
-- The key was (entity_type, requester_id, normalized name) — the name ALONE
-- (20260607015616_entity_requests_fulfillment_source_dedup). A recurring night
-- is the domain's normal case, so one requester queueing "Open Mic" for two
-- dates collided on that key, and since PSY-1948 the collision REPLACES the
-- queued row's payload: the first night's request was destroyed. The key now
-- carries the payload's occurrence date, so those are two requests.
--
-- The occurrence term is the payload's REQUIRED date field, coalesced across the
-- two entity types that have one: show carries event_date, festival carries
-- start_date. Each payload type NAMES its own occurrence field in Go, on the
-- closed EntityRequestPayload interface (dedupOccurrenceJSONKey), which is where
-- the per-type reasoning lives and why release_date is deliberately absent from
-- the coalesce below. Those declarations and this expression are pinned to each
-- other by TestDedupOccurrenceExprReadsEveryRegisteredKey.
--
-- The venue does NOT discriminate, and cannot: ShowRequestPayload has no venue
-- field at all (the approving admin supplies the venue at fulfillment), so there
-- is nothing in the payload to key on.
--
-- '' is the default rather than NULL because a NULL key column makes every row
-- DISTINCT under a Postgres unique index. Left as NULL, this term would silently
-- disable dedup entirely for artist, venue, label and release — the types whose
-- payloads have no date. Empty string collapses them all into one bucket, which
-- is exactly the title-only key they had before.
--
-- EXISTING ROWS: none need resolving. The new key is strictly WIDER than the old
-- one, so every set of rows that satisfied the old index satisfies this one; the
-- build cannot fail on data that is already there. What survives is rows that
-- were previously prevented — none exist yet, because the old index prevented
-- them.
--
-- Multi-statement file ⇒ golang-migrate wraps it in a transaction, so these are
-- plain (NOT CONCURRENTLY) statements. entity_requests is a small moderation
-- queue, so a transactional index rebuild does not lock meaningful write traffic.

DROP INDEX IF EXISTS uq_entity_requests_pending_dedup;

CREATE UNIQUE INDEX uq_entity_requests_pending_dedup
    ON entity_requests (
        entity_type,
        requester_id,
        (lower(trim(coalesce(payload->>'name', payload->>'title')))),
        (trim(coalesce(payload->>'event_date', payload->>'start_date', '')))
    )
    WHERE decision_state = 'pending';
