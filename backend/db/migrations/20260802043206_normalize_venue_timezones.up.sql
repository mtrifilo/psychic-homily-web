-- PSY-1707: bring venues.timezone up to the invariant the write boundary now
-- holds -- every stored value is a name Postgres itself recognizes.
--
-- Readers resolve this column with AT TIME ZONE, which does NOT degrade
-- gracefully: an unrecognized name raises and takes the whole query down. That
-- is the reason PSY-1695 cannot simply trust the column without this.
--
-- MEASURED BEFORE WRITING THIS: both environments are already clean.
--   stage      161 venues -- 153 set, 8 NULL, 0 invalid, 0 non-canonical
--   production 237 venues -- 229 set, 8 NULL, 0 invalid, 0 non-canonical
-- (read-only SELECTs against pg_timezone_names, 2026-08-02)
--
-- So this is expected to update ZERO rows. It ships anyway for two reasons: it
-- closes the window between that audit and the deploy, and it makes the
-- invariant true by construction for any environment restored from an older
-- dump or seeded by hand.

-- 1. Canonicalize spelling/whitespace where the value resolves but is not
--    stored exactly as pg_timezone_names spells it ("america/phoenix",
--    " America/Phoenix "). Matching stays case-insensitive at read time, but
--    storing the canonical form keeps equality comparisons honest.
-- A correlated subquery with an explicit ORDER BY, not a join to
-- pg_timezone_names. UPDATE ... FROM picks an arbitrary row when the join
-- matches more than one, and "lower(name) is unique" is a property of the host
-- OS tzdata, not something this migration can assume. It holds on the current
-- image (verified: zero rows from
-- `SELECT lower(name) FROM pg_timezone_names GROUP BY 1 HAVING count(*) > 1`),
-- but a silent arbitrary pick is not a thing to leave in a migration.
UPDATE venues v
SET timezone = (
  SELECT t.name FROM pg_timezone_names t
  WHERE lower(t.name) = lower(btrim(v.timezone))
  ORDER BY t.name
  LIMIT 1
)
WHERE v.timezone IS NOT NULL
  AND btrim(v.timezone) <> ''
  AND EXISTS (
    SELECT 1 FROM pg_timezone_names t
    WHERE lower(t.name) = lower(btrim(v.timezone))
      AND t.name <> v.timezone
  );

-- 2. NULL anything Postgres cannot resolve at all, including blank strings.
--    NULL is the shape every reader already handles -- it is what a geocode
--    MISS produces -- and it falls back to the US state map. Deliberately not
--    "guess a replacement": a wrong zone is worse than an absent one, because
--    absent is visibly a gap while wrong silently mis-dates every show.
UPDATE venues v
SET timezone = NULL
WHERE v.timezone IS NOT NULL
  AND (
    btrim(v.timezone) = ''
    OR NOT EXISTS (
      SELECT 1 FROM pg_timezone_names t
      WHERE lower(t.name) = lower(btrim(v.timezone))
    )
  );
