ALTER TABLE venues
    DROP COLUMN IF EXISTS street_latitude,
    DROP COLUMN IF EXISTS street_longitude,
    DROP COLUMN IF EXISTS geocode_precision,
    DROP COLUMN IF EXISTS geocoded_address;
