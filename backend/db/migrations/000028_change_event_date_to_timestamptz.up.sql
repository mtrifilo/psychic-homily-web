-- Change event_date from TIMESTAMP to TIMESTAMPTZ.
-- The existing values were stored as UTC, so we interpret them as UTC during conversion.
-- This fixes cursor pagination where pgx sends time.Time as timestamptz parameters
-- but the column was timestamp, causing equality comparisons to fail.
ALTER TABLE shows ALTER COLUMN event_date TYPE TIMESTAMPTZ USING event_date AT TIME ZONE 'UTC';
