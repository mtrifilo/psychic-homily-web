-- Add deletion tracking fields to users table for soft delete with grace period
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE users ADD COLUMN deletion_reason VARCHAR(500) NULL;

-- Index for efficiently finding deleted users (for cleanup jobs)
CREATE INDEX idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;
