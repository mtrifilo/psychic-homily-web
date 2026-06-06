-- Add geocoding columns to venues for venue-anchored timezones (PSY-985).
-- `timezone` is the IANA zone (e.g. America/Phoenix, Europe/London) resolved
-- from the city at venue creation; latitude/longitude are the geocoded city
-- centroid. All nullable: existing rows are backfilled separately (PSY-987),
-- and a geocode miss leaves them NULL so callers fall back to the legacy
-- state->timezone map.
ALTER TABLE venues
    ADD COLUMN latitude NUMERIC(9,6),
    ADD COLUMN longitude NUMERIC(9,6),
    ADD COLUMN timezone TEXT;
