-- The lookups are the only record of which addresses the backfill wrote, so
-- they are kept as venue_address_lookups_archive rather than dropped; a later
-- rollback adds its rows to an archive that already exists.
DO $$
BEGIN
    IF to_regclass('venue_address_lookups') IS NULL THEN
        RETURN;
    END IF;
    IF to_regclass('venue_address_lookups_archive') IS NULL THEN
        ALTER TABLE venue_address_lookups RENAME TO venue_address_lookups_archive;
        -- Index names are schema-wide; free the primary key name for a later up.
        ALTER INDEX venue_address_lookups_pkey RENAME TO venue_address_lookups_archive_pkey;
    ELSE
        INSERT INTO venue_address_lookups_archive
        SELECT * FROM venue_address_lookups
        ON CONFLICT DO NOTHING;
        DROP TABLE venue_address_lookups;
    END IF;
END $$;

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
