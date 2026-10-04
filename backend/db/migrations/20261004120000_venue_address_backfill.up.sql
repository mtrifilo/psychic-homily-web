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
-- was tried (the candidate page URLs for 'page', the search text for 'name');
-- a 'miss' whose key still matches the venue's current inputs is skipped on
-- the next run, and a changed key (new website, renamed venue) is tried again.
-- A 'hit' keeps the source the address came from.
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
