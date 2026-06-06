ALTER TABLE venues
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude;
