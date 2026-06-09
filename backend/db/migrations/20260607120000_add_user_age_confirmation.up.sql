-- PSY-1023: record an explicit "I am at least N years old" confirmation at
-- signup. Mirrors the terms-acceptance evidence columns added in 000031.
-- age_confirmed_at is the acceptance timestamp; min_age_attested records the
-- minimum age the user attested to (e.g. 16) so the value survives a future
-- change to the minimum.
ALTER TABLE users
  ADD COLUMN age_confirmed_at TIMESTAMPTZ,
  ADD COLUMN min_age_attested INTEGER;
