-- PSY-1570: expression indexes for radio_normalize_name() lookups.
--
-- These predicates accounted for 91% of ALL query time on production.
-- radio_normalize_name is IMMUTABLE, so it is indexable; nothing indexed it,
-- so every lookup sequentially scanned a 238k-row table.
--
-- STEP 1 IS A PREREQUISITE, NOT A CLEANUP. radio_normalize_name calls
-- immutable_unaccent UNQUALIFIED. Index expressions are evaluated under a
-- restricted search_path (Postgres does this deliberately, so an index
-- expression cannot be hijacked by a schema earlier in a caller's path), and
-- under that path an unqualified public function is not visible. Creating an
-- index on the function as written fails outright:
--
--   ERROR: function immutable_unaccent(text) does not exist
--   CONTEXT: SQL function "radio_normalize_name" during inlining
--
-- Verified against Postgres 17.7 (prod's version) on a 238,125-row
-- reproduction: the CREATE INDEX below fails without step 1 and succeeds with
-- it. immutable_unaccent already qualifies public.unaccent, so this is the one
-- remaining unqualified hop. Behaviour is otherwise identical — same inputs,
-- same outputs; only name resolution changes.
--
-- The normalization PIPELINE itself is unchanged and still must stay in sync
-- with normalizeName in radio_matching.go — see 20260711140000 (PSY-1441) for
-- the contract and why interior punctuation is preserved (AC/DC != ACDC).
CREATE OR REPLACE FUNCTION public.radio_normalize_name(text)
    RETURNS text
    LANGUAGE sql
    IMMUTABLE PARALLEL SAFE STRICT
AS $function$
    SELECT regexp_replace(
      regexp_replace(
        regexp_replace(public.immutable_unaccent(LOWER($1)), '^[^a-z0-9]+', ''),
        '[^a-z0-9]+$',
        ''
      ),
      '\s+',
      ' ',
      'g'
    )
  $function$;

-- STEP 2: the indexes.

-- Hot query (2287 calls, mean 1473ms, ~56 min total DB time), from
-- MatchUnmatchedPlaysForArtistName:
--   SELECT * FROM radio_plays
--    WHERE artist_id IS NULL AND match_state IN (...)
--      AND radio_normalize_name(artist_name) = $1
--
-- Partial on artist_id IS NULL ONLY. That clause is common to both the
-- force=true and force=false callers, so one index serves both — measured, not
-- assumed: both plans below use it. match_state is deliberately NOT a second
-- column, because force=true passes all three states, i.e. every one of the
-- 218,857 unlinked rows; including it would add size and buy nothing.
--
-- Note this does NOT supersede idx_radio_plays_unmatched_artist_name
-- (PSY-1366). That index is on the RAW column for chunked distinct-name
-- paging, and the reproduction showed the planner still choosing it for the
-- single-match_state shape. It reports 0 scans on prod only because the hot
-- caller is force=true. Do not drop it on that number alone.
--
-- Measured on the 238,125-row reproduction (Postgres 17.7):
--   force=true   127.258 ms (parallel seq scan, 1905 buffers) -> 0.067 ms (9 buffers)
--   force=false   85.983 ms                                   -> 0.040 ms
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_radio_plays_norm_artist_name
    ON radio_plays (radio_normalize_name(artist_name))
    WHERE artist_id IS NULL;

-- 6118 calls, mean 120ms, ~12 min total.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_releases_norm_title
    ON releases (radio_normalize_name(title));

-- 8714 calls, mean 22ms; also serves the id/slug-only variant of the lookup.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_artists_norm_name
    ON artists (radio_normalize_name(name));

-- Deliberately NOT indexed: labels (24 rows) and artist_aliases (10 rows) use
-- the same function but already run at 0.3ms and 0.2ms mean. A seq scan is the
-- correct plan at that size; an index would be overhead with no upside.
