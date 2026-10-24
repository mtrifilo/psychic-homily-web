SET LOCAL lock_timeout = '5s';

ALTER TABLE users
    DROP COLUMN IF EXISTS sessions_valid_after;
