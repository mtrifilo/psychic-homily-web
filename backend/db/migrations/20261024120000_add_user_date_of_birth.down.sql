SET LOCAL lock_timeout = '5s';

ALTER TABLE users
    DROP COLUMN IF EXISTS age_attested_at,
    DROP COLUMN IF EXISTS date_of_birth;
