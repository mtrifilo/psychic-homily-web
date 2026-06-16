-- Reverse PSY-1115: drop the nav_mode column (its CHECK constraint drops with
-- it). Restores the pre-migration users shape for the up->down->up round-trip.
ALTER TABLE users DROP COLUMN nav_mode;
