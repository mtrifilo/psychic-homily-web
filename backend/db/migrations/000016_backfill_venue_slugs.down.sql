-- Revert slugs (set back to NULL)
-- Note: This is a data migration, down just clears the slugs
UPDATE venues SET slug = NULL;
UPDATE artists SET slug = NULL;
