-- Revert the unique index on (station_id, external_id).
-- We do NOT revert the external_id fixes or duplicate cleanup — those are
-- data corrections that should persist even if the migration is rolled back.
DROP INDEX IF EXISTS idx_radio_shows_station_external_id;
