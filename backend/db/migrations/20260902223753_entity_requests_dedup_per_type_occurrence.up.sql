-- PSY-1989: make the pending-request dedup key's OCCURRENCE term per entity
-- type, so venue and festival requests stop destroying each other.
--
-- PSY-1977 (20260831065648) gave the key an occurrence, but only shows declared
-- one, so every other type still keyed on the name alone: a second pending
-- request under that name REPLACED the first (PSY-1948), destroying it. Two
-- Fillmores in two cities, and two editions of one festival, are the ordinary
-- cases. The occurrence is now a CASE over entity_type, and each type's term is
-- declared beside its payload struct in Go
-- (EntityRequestPayload.dedupOccurrenceTerm), which is where the per-type
-- reasoning lives. TestDedupOccurrenceMatchesTheIndex reads THIS index out of
-- Postgres and asserts it against those declarations, so the two cannot drift.
--
-- EACH TERM IS READ OFF THE CATALOG CONSTRAINT THE FULFILLED ENTITY MEETS. A key
-- FINER than the catalog's uniqueness is not a fix: it files two requests an
-- admin can approve, and the second fulfilment then fails AFTER its row has been
-- claimed, leaving an approved-but-unfulfilled row for the rescue flow. A
-- destroyed request traded for an orphaned one is not an improvement.
--
--   venue     city, case-folded. venues is UNIQUE (LOWER(name), LOWER(city)) and
--             CreateVenue refuses a second venue with that pair, so the key
--             matches it exactly. STATE IS DELIBERATELY NOT IN THE KEY: the
--             catalog does not key on it, so a state term would separate two
--             rows the catalog merges. city is required on the payload.
--   festival  the edition YEAR: the stated edition_year, else the year off the
--             front of the required start_date, which is the derivation
--             fulfillment performs. festivals is UNIQUE (series_slug,
--             edition_year), so start_date itself would be finer than the
--             catalog. '0' is read as "not stated" because that is what a
--             marshalled zero value looks like and what fulfillment treats as
--             absent; without that, one festival keys two ways depending on
--             whether the client omitted the field or sent 0.
--   show      event_date, unchanged from PSY-1977, and NOT case-folded (see
--             below).
--
-- THREE TYPES STILL KEY ON THE NAME ALONE, and the reasons differ:
--   artist    CORRECT, not a gap. artists_lower_name_uniq makes an artist name
--             globally unique, case-insensitively, so two same-named artist
--             requests are one catalog artist.
--   label     UNFIXED. CreateLabel suffixes a taken slug, so two same-named
--             labels really can both exist, but every field that would separate
--             them (city, state, country, founded_year) is optional, and an
--             optional term turns "queued without it, resubmitted with it" into
--             a second request rather than the correction it is.
--   release   UNFIXED, and unfixable from this payload: what separates two
--             same-titled releases is the ARTIST, and the payload has no artist
--             field at all.
--
-- SHOWS ARE STILL NOT FULLY FIXED: same title, same date, DIFFERENT VENUE still
-- collides, because the payload carries no venue.
--
-- EXISTING ROWS: uniqueness cannot fail; SIZE needs a preflight. Read both.
--
-- UNIQUENESS CANNOT FAIL because this key is strictly WIDER than the one it
-- replaces, term by term. venue and festival go from a constant '' to a value,
-- which only splits buckets. artist, label and release keep the constant ''.
-- The show branch is the same expression rewritten
-- (left(trim(coalesce(x,'')),64) became left(coalesce(nullif(trim(x),''),''),64),
-- which agree on every input, empty and whitespace included). The venue term is
-- the one that is case-FOLDED, and folding narrows — but it folds a term that
-- was the constant '' until this migration, so it cannot merge two rows that the
-- old index kept apart. event_date is deliberately NOT folded for exactly this
-- reason: it is the one term already in production, and folding it could newly
-- collide two pending rows differing only in the case of an RFC3339 T or Z.
--
-- SIZE CAN FAIL. A btree index tuple has a hard ~2704-byte limit, and this
-- migration ADDS UP TO 255 BYTES (a venue's city) to that tuple, so "it fit the
-- previous index" is not a bound on whether it fits this one. The name term is
-- still indexed UNTRUNCATED and, on rows queued before PSY-1990, still uncapped:
-- a pending row with a ~2.6 KB name is what aborted the PSY-1977 build with
-- SQLSTATE 54000. PSY-1990 caps name/title at 255 at the API boundary in this
-- same PR, which bounds every NEW row but does nothing for rows already queued.
--
-- PREFLIGHT before deploying this, and resolve anything it returns by shortening
-- or deciding the row:
--
--   SELECT id, entity_type, requester_id,
--          octet_length(lower(trim(coalesce(payload->>'name', payload->>'title'))))
--            + octet_length(coalesce(payload->>'city', '')) AS key_bytes
--     FROM entity_requests
--    WHERE decision_state = 'pending'
--      AND octet_length(lower(trim(coalesce(payload->>'name', payload->>'title'))))
--          + octet_length(coalesce(payload->>'city', '')) > 2400
--    ORDER BY key_bytes DESC;
--
-- If it fails anyway, the multi-statement transaction rolls back (no data lost)
-- and golang-migrate leaves schema_migrations dirty at THIS migration's own
-- version: recover with `migrate force 20260831065648` and then `up` once the
-- row is shortened.
--
-- ROLLING THIS BACK: run the down migration and deploy the pre-PSY-1989 build
-- TOGETHER, in that order. A pre-PSY-1989 binary against this index is a broken
-- pair for venues and festivals: its lookup keys on the name alone, so it can
-- select a row this INSERT never collided with, and the replacement it then
-- attempts violates this index. The contributor's correction 500s
-- deterministically until the rows are resolved; it does not destroy the other
-- row, because this index refuses that write. See the down migration.
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
        (CASE entity_type
            WHEN 'festival' THEN left(coalesce(nullif(nullif(trim(payload->>'edition_year'), ''), '0'), nullif(trim(payload->>'start_date'), ''), ''), 4)
            WHEN 'show' THEN left(coalesce(nullif(trim(payload->>'event_date'), ''), ''), 64)
            WHEN 'venue' THEN lower(left(coalesce(nullif(trim(payload->>'city'), ''), ''), 255))
            ELSE ''
        END)
    )
    WHERE decision_state = 'pending';
