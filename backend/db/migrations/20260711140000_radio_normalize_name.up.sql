-- PSY-1441: Mirror Go normalizeName boundary-trim + whitespace collapse in SQL
-- so KG canonical names with trailing/leading punctuation (ADULT., !Calhau!,
-- P.I.L.) match plays that fold to the same form.
--
-- Depends on immutable_unaccent from 20260528160326_radio_matching_unaccent_indexes.
-- Pipeline (must stay in sync with normalizeName in radio_matching.go):
--   1. immutable_unaccent(LOWER(text))  — diacritic + case fold
--   2. strip leading non [a-z0-9]
--   3. strip trailing non [a-z0-9]
--   4. collapse interior whitespace runs to a single ASCII space
--
-- Interior punctuation is intentionally preserved (AC/DC ≠ ACDC).

CREATE OR REPLACE FUNCTION radio_normalize_name(text)
  RETURNS text
  AS $$
    SELECT regexp_replace(
      regexp_replace(
        regexp_replace(immutable_unaccent(LOWER($1)), '^[^a-z0-9]+', ''),
        '[^a-z0-9]+$',
        ''
      ),
      '\s+',
      ' ',
      'g'
    )
  $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE STRICT;
