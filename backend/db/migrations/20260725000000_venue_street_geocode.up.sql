-- PSY-1536: street-level venue geocoding via Nominatim/OSM.
-- street_latitude/street_longitude hold the geocoded coordinates of
-- venues.address. The existing latitude/longitude columns stay untouched —
-- they are CITY CENTROIDS from the offline GeoNames geocoder, and scenes,
-- metro rollup, and timezone derivation depend on them.
-- geocode_precision records how precise the hit was; geocoded_address stores
-- the exact address key that produced the coordinates, so unchanged addresses
-- are skipped on backfill re-runs and a stale geocode (address changed since)
-- is detectable by comparison.
ALTER TABLE venues
    ADD COLUMN street_latitude NUMERIC(9,6),
    ADD COLUMN street_longitude NUMERIC(9,6),
    ADD COLUMN geocode_precision VARCHAR(20)
        CONSTRAINT venues_geocode_precision_check
        CHECK (geocode_precision IN ('rooftop', 'interpolated', 'city')),
    ADD COLUMN geocoded_address TEXT;
