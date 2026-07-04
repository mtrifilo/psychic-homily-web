ALTER TABLE user_preferences DROP COLUMN IF EXISTS notify_on_scene_digest;
ALTER TABLE user_bookmarks DROP COLUMN IF EXISTS scene_digest_sent_at;
