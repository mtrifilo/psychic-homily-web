-- Note: PostgreSQL doesn't support removing enum values directly.
-- This migration cannot be fully reversed without recreating the enum type.
-- Shows with 'private' status should be deleted or updated before rolling back.

-- Update any private shows to pending before attempting rollback
UPDATE shows SET status = 'pending' WHERE status = 'private';

-- Note: The enum value 'private' will remain in the type definition
-- as PostgreSQL doesn't support DROP VALUE from enums.
-- A full rollback would require recreating the enum type.
