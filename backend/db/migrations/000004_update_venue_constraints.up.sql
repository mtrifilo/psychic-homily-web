-- Update venue constraints: make city/state required, add verification, and enforce uniqueness per city

-- First, ensure any NULL values have defaults (if any exist)
UPDATE venues SET city = 'Unknown' WHERE city IS NULL;
UPDATE venues SET state = 'XX' WHERE state IS NULL;

-- Make city and state required
ALTER TABLE venues ALTER COLUMN city SET NOT NULL;
ALTER TABLE venues ALTER COLUMN state SET NOT NULL;

-- Add verified column for admin approval (default false for NEW venues going forward)
ALTER TABLE venues ADD COLUMN verified BOOLEAN NOT NULL DEFAULT false;

-- Mark all EXISTING venues as verified (they're already vetted)
UPDATE venues SET verified = true;

-- Drop the existing unique constraint on venues.name (GORM names it {table}_{column}_key)
ALTER TABLE venues DROP CONSTRAINT IF EXISTS venues_name_key;

-- Create a composite unique index on (name, city) to allow same names in different cities
CREATE UNIQUE INDEX idx_venues_name_city_unique ON venues(LOWER(name), LOWER(city));

-- Add comments for documentation
COMMENT ON COLUMN venues.city IS 'City where the venue is located (required)';
COMMENT ON COLUMN venues.state IS 'State/province where the venue is located (required)';
COMMENT ON COLUMN venues.verified IS 'Admin verification flag - true if venue is confirmed legitimate and safe';
COMMENT ON INDEX idx_venues_name_city_unique IS 'Ensures venue names are unique within a city, allowing same name in different cities';

