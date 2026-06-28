-- PSY-1255 step B: denormalized CBSA metro key for the Atlas scene rollup.
--
-- `metro` stores the US Census CBSA code (e.g. "35620" = New York-Newark-Jersey
-- City) that the entity's home (city, state, country) resolves to via the offline
-- geocoder (geo.ResolveMetro). It lets a scene group every artist/venue BASED in
-- one metro — boroughs and suburbs included — by an indexed equality match,
-- instead of re-geocoding per query. The value is DERIVED: it is reconciled by
-- cmd/backfill-entity-metro and set on the create write paths. NULL for non-US,
-- not-in-any-CBSA, and ambiguous-unpinned places (geo.ResolveMetro returns ok=false).
ALTER TABLE artists ADD COLUMN metro VARCHAR(10);
ALTER TABLE venues  ADD COLUMN metro VARCHAR(10);

-- Partial indexes: the scene roster query matches `metro = $cbsaCode`, which never
-- matches NULL, so excluding the (large) NULL tail keeps the index small. Inlined
-- (not CONCURRENTLY) because this migration is multi-statement/transactional and
-- artists/venues are small enough today that the build lock is negligible
-- (mirrors PSY-413/PSY-886; revisit with CONCURRENTLY if these tables grow large).
CREATE INDEX idx_artists_metro ON artists (metro) WHERE metro IS NOT NULL;
CREATE INDEX idx_venues_metro  ON venues  (metro) WHERE metro IS NOT NULL;
