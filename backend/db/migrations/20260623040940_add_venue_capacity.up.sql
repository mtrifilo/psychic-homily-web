-- PSY-1179: persist venue capacity captured during ingest.
--
-- The `ph` CLI venue ingest carries a `capacity` (e.g. from a venue's own page),
-- but there was no column to store it, so it was silently dropped on create AND
-- could not be dedup-compared on re-ingest (forcing a spurious UPDATE every run —
-- the reason PSY-1171 had to drop capacity from the comparison). This adds the
-- column so create + dedup-enrichment round-trip it. Exposed read/write on the
-- venue create/update/detail contracts (capacity is not sensitive, so unlike
-- address/zipcode it is NOT redacted for unverified venues).
--
-- ADDITIVE: a single nullable INTEGER with no DEFAULT => Postgres adds it without
-- a table rewrite. Existing rows keep NULL capacity (unknown).
ALTER TABLE venues
    ADD COLUMN capacity INTEGER;
