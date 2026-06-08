ALTER TABLE users
  DROP COLUMN IF EXISTS min_age_attested,
  DROP COLUMN IF EXISTS age_confirmed_at;
