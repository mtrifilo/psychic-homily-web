-- Revert PSY-1045 privacy-default flip: restore the pre-flip column default
-- and walk rows that exactly match the NEW default back to the old default.
-- (Users who customized after the flip keep their stored settings — same
-- verbatim-equality posture as the up migration, so up→down→up round-trips.)

ALTER TABLE users ALTER COLUMN privacy_settings SET DEFAULT '{"contributions":"visible","saved_shows":"hidden","attendance":"hidden","following":"count_only","collections":"visible","last_active":"visible","profile_sections":"visible"}';

UPDATE users
SET privacy_settings = '{"contributions":"visible","saved_shows":"hidden","attendance":"hidden","following":"count_only","collections":"visible","last_active":"visible","profile_sections":"visible"}'::jsonb
WHERE privacy_settings = '{"contributions":"visible","saved_shows":"hidden","attendance":"visible","following":"visible","collections":"visible","last_active":"visible","profile_sections":"visible"}'::jsonb;
