DROP TABLE IF EXISTS venue_address_lookups;

-- The narrower constraint cannot hold 'name_search' rows. Their addresses stay;
-- the street geocode is cleared so the street-geocode sweep re-resolves each
-- address through the structured search and labels it with the old vocabulary.
UPDATE venues
SET street_latitude = NULL,
    street_longitude = NULL,
    geocode_precision = NULL,
    geocoded_address = NULL
WHERE geocode_precision = 'name_search';

ALTER TABLE venues DROP CONSTRAINT IF EXISTS venues_geocode_precision_check;
ALTER TABLE venues
    ADD CONSTRAINT venues_geocode_precision_check
    CHECK (geocode_precision IN ('rooftop', 'interpolated', 'city'));
