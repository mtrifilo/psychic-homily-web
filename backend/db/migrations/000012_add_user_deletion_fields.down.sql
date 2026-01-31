-- Remove deletion tracking fields from users table
DROP INDEX IF EXISTS idx_users_deleted_at;
ALTER TABLE users DROP COLUMN IF EXISTS deletion_reason;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
