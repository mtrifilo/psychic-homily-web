-- PSY-1416: optional free-text profile location ("City, state").
-- Nullable + additive; no geocoding. Mirrors display_name length (VARCHAR(100)).
ALTER TABLE users ADD COLUMN location VARCHAR(100);
