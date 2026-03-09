DROP TABLE IF EXISTS user_profile_sections;
DROP INDEX IF EXISTS idx_users_user_tier;
ALTER TABLE users DROP COLUMN IF EXISTS user_tier;
ALTER TABLE users DROP COLUMN IF EXISTS privacy_settings;
