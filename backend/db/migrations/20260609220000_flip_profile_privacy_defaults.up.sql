-- PSY-1045: flip profile privacy defaults — following: count_only → visible,
-- attendance: hidden → visible. The content-first profile leads with what a
-- user follows and the shows they've attended, so the defaults expose them
-- (2026-06-09 product decision).
--
-- privacy_settings is NOT NULL with a DB default, so every row carries
-- explicit JSON. "Never customized" is therefore identified as rows whose
-- JSON still equals the old default verbatim (jsonb equality is key-order
-- insensitive). Users who changed ANY field keep their stored posture.

ALTER TABLE users ALTER COLUMN privacy_settings SET DEFAULT '{"contributions":"visible","saved_shows":"hidden","attendance":"visible","following":"visible","collections":"visible","last_active":"visible","profile_sections":"visible"}';

UPDATE users
SET privacy_settings = '{"contributions":"visible","saved_shows":"hidden","attendance":"visible","following":"visible","collections":"visible","last_active":"visible","profile_sections":"visible"}'::jsonb
WHERE privacy_settings = '{"contributions":"visible","saved_shows":"hidden","attendance":"hidden","following":"count_only","collections":"visible","last_active":"visible","profile_sections":"visible"}'::jsonb;
