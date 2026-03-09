DROP INDEX IF EXISTS idx_users_profile_visibility;
ALTER TABLE users DROP COLUMN IF EXISTS profile_visibility;
