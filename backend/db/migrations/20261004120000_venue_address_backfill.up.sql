-- Venue address backfill: a venue with no address can be given one from its
-- own web page or from an OpenStreetMap place found by searching its name.
--
-- geocode_precision gains 'name_search': the street point and the address
-- were both taken from the OSM place a name search matched, rather than from
-- resolving an address the venue already carried.
ALTER TABLE venues DROP CONSTRAINT IF EXISTS venues_geocode_precision_check;
ALTER TABLE venues
    ADD CONSTRAINT venues_geocode_precision_check
    CHECK (geocode_precision IN ('rooftop', 'interpolated', 'city', 'name_search'));

-- One row per venue per backfill phase: the last attempt. lookup_key is what
-- was tried (the candidate pages for 'page', the search for 'name'); a 'miss'
-- whose key still matches the venue's current inputs is skipped on the next
-- run, and a changed key (new website, renamed venue) is tried again. A 'hit'
-- keeps the source the address came from, and marks the venue as filled once,
-- so an address cleared later is never refilled.
--
-- The down migration keeps these rows as venue_address_lookups_archive; when
-- that table exists it is restored here instead of starting empty, so a
-- rollback and re-apply loses neither the misses nor the record of fills.
DO $$
BEGIN
    IF to_regclass('venue_address_lookups_archive') IS NOT NULL THEN
        ALTER TABLE venue_address_lookups_archive RENAME TO venue_address_lookups;
        ALTER INDEX IF EXISTS venue_address_lookups_archive_pkey RENAME TO venue_address_lookups_pkey;
    ELSE
        CREATE TABLE venue_address_lookups (
            venue_id     INTEGER     NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
            phase        VARCHAR(10) NOT NULL CHECK (phase IN ('page', 'name')),
            lookup_key   TEXT        NOT NULL,
            outcome      VARCHAR(10) NOT NULL CHECK (outcome IN ('hit', 'miss')),
            source       TEXT,
            address      TEXT,
            attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            PRIMARY KEY (venue_id, phase)
        );
    END IF;
END $$;
