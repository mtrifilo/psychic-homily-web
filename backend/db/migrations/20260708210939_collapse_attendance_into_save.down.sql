-- IRREVERSIBLE (data): the going/interested distinction is destroyed by the up
-- migration. Rows were relabelled to 'save' and duplicate rows deleted; nothing
-- records which of the three actions a surviving row originally carried. There
-- is no correct way to restore them, so this down migration deliberately does
-- NOT attempt to split 'save' rows back apart — a guess would silently
-- manufacture attendance data that no user ever entered.
--
-- What IS restored: the schema-level artifacts, so a rollback leaves the column
-- default and table comments matching the pre-migration state.

ALTER TABLE users ALTER COLUMN privacy_settings SET DEFAULT '{"contributions":"visible","saved_shows":"hidden","attendance":"visible","following":"visible","collections":"visible","last_active":"visible","profile_sections":"visible"}';

UPDATE users
SET privacy_settings = privacy_settings || '{"attendance":"visible"}'::jsonb
WHERE privacy_settings IS NOT NULL
  AND NOT jsonb_exists(privacy_settings, 'attendance');

COMMENT ON TABLE user_bookmarks IS 'Generic user-entity relationship table supporting saves, follows, bookmarks, going, interested actions across all entity types';
COMMENT ON COLUMN user_bookmarks.action IS 'Action type: save, follow, bookmark, going, interested';
