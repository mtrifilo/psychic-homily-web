-- A snapshot of this server's zone catalog, so a listing query can test a
-- stored venues.timezone WITHOUT reading pg_timezone_names on the hot path.
--
-- Why a copy rather than the catalog itself: pg_timezone_names is a set-
-- returning function that reopens the tzdata files on every scan. Measured on
-- postgres:18 with the /shows feed's own queries, one scan costs 3.5 ms idle
-- and 43-63 ms under load, against a 4.9 ms budget for the whole upcoming-shows
-- count. A seq scan of this table costs 0.1-0.5 ms for the same 487 rows.
--
-- Seeded HERE rather than by a background job on purpose. The read path depends
-- on this table being populated, so populating it belongs to the schema: a
-- migrated database is a correct one, with no flag to set and no worker to
-- remember. catalog.RefreshTimezoneNamesSnapshot keeps it current afterwards,
-- because what pg_timezone_names contains is a property of the server's tzdata
-- PACKAGING and changes under a Postgres upgrade or a restore onto a
-- differently-packaged image (measured: postgres:18 Debian carries 487 zones,
-- postgres:16-alpine 599).
--
-- name_lower is generated rather than maintained, and it is what the guard
-- compares against: AT TIME ZONE is case-insensitive and so is the venue
-- timezone sweep, so a case-insensitive membership test is the one that agrees
-- with both. Comparing the stored spelling exactly would send a venue holding
-- 'america/phoenix' to the state-map fallback that AT TIME ZONE would have
-- accepted -- safe, but a silent mislabel this table exists to avoid.
CREATE TABLE IF NOT EXISTS timezone_names_snapshot (
    name       TEXT PRIMARY KEY,
    name_lower TEXT GENERATED ALWAYS AS (lower(name)) STORED
);

INSERT INTO timezone_names_snapshot (name)
SELECT name FROM pg_timezone_names
ON CONFLICT (name) DO NOTHING;
