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
-- SCOPE: shows only. event_date is the sole key in the coalesce below, and
-- ShowRequestPayload is the only type that declares one. Each payload type names
-- its own occurrence field in Go, on the closed EntityRequestPayload interface
-- (dedupOccurrenceJSONKey), which is where the per-type reasoning lives —
-- including why release, venue and festival deliberately declare none, each of
-- which leaves a same-name collision this migration does NOT fix.
-- TestDedupOccurrenceMatchesTheIndex reads THIS index out of Postgres and
-- asserts it against those declarations, so the two cannot drift.
--
-- The venue does not discriminate: a show payload's only geographic fields are
-- city and state, both OPTIONAL, and an optional field cannot go in the key (see
-- the interface's rule). The venue proper is supplied by the approving admin at
-- fulfillment and is not in the payload at all. Two same-titled shows on the same
-- date in different cities therefore STILL collide destructively — a franchise
-- night running two cities the same evening is the real case, and it is out of
-- this ticket's scope rather than solved by it.
--
-- The term is the payload's event_date STRING, compared byte for byte after
-- trimming. It is NOT an instant comparison, and it is deliberately not described
-- as one: the catalog's own show dedup key IS an instant (PSY-559,
-- catalog/show.go), and this key does not partition the same way. Two spellings
-- of one moment — "2026-09-03" vs "2026-09-03T20:00:00-07:00", or a -07:00 offset
-- vs the equivalent Z — are different buckets here and the same show downstream.
-- The cost is a second queued row rather than a correction, and if an admin
-- approves both, the second fulfillment fails the catalog's duplicate check after
-- the row is already claimed, leaving an approved-but-unfulfilled row for the
-- rescue flow. Canonicalizing event_date at the boundary would close this, but a
-- date-only value is anchored at 20:00 VENUE-LOCAL at fulfillment and the venue
-- is unknown at submit, so it cannot simply be normalized to an instant here.
-- That is an open decision, deliberately not guessed at in this migration.
--
-- '' is the default rather than NULL because a NULL key column makes every row
-- DISTINCT under a Postgres unique index. Left as NULL, this term would silently
-- disable dedup entirely for every type whose payload has no event_date. Empty
-- string collapses them all into one bucket, which is exactly the title-only key
-- they had before.
--
-- EXISTING ROWS: none need resolving. The new key is strictly WIDER than the old
-- one, so every set of rows that satisfied the old index satisfies this one; the
-- build cannot fail on data that is already there. What survives is rows that
-- were previously prevented — none exist yet, because the old index prevented
-- them.
--
-- ROLLING THIS BACK: run the down migration and deploy the pre-PSY-1977 build
-- TOGETHER. A pre-PSY-1977 binary against this WIDE index is a broken pair: its
-- name-only lookup can select a row the INSERT never collided with, and the
-- replacement it then attempts violates this index, so the contributor's
-- correction 500s deterministically until the rows are resolved. It does not
-- destroy the other row — this index refuses that write, verified against a live
-- Postgres — but the endpoint is unusable for that name. See the down migration.
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
        (trim(coalesce(payload->>'event_date', '')))
    )
    WHERE decision_state = 'pending';
