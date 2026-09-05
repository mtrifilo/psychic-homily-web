-- Reverse PSY-1706's venues.capacity range constraint.
--
-- Non-destructive: dropping a CHECK constraint neither reads nor rewrites rows,
-- and every value that satisfied it still satisfies the column's type. The
-- API-layer bounds are unaffected and keep refusing out-of-range writes.

ALTER TABLE venues
    DROP CONSTRAINT IF EXISTS venues_capacity_range;

COMMENT ON COLUMN venues.capacity IS NULL;
