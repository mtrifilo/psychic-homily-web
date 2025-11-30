-- Revert venue constraint changes

-- Drop the composite unique index
DROP INDEX IF EXISTS idx_venues_name_city_unique;

-- Restore the original unique constraint on name only
-- Note: This may fail if there are duplicate names in the database
ALTER TABLE venues ADD CONSTRAINT venues_name_key UNIQUE (name);

-- Revert city and state to nullable
ALTER TABLE venues ALTER COLUMN city DROP NOT NULL;
ALTER TABLE venues ALTER COLUMN state DROP NOT NULL;

-- Remove verified column
ALTER TABLE venues DROP COLUMN IF EXISTS verified;

