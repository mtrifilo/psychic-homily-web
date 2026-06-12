-- PSY-1063: explicit display name, preferred over first/last in the
-- attribution resolution chain. Nullable + additive; first_name/last_name
-- remain populated for existing rows.
ALTER TABLE users ADD COLUMN display_name VARCHAR(100);
