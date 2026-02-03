-- Revert show slugs (set back to NULL)
-- Note: This is a data migration, down just clears the slugs
UPDATE shows SET slug = NULL;
