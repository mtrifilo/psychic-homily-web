-- Reverse 20261011120000_create_radio_play_exclusions.up.sql. DROP TABLE removes
-- the index, constraints and FK with it, and every exclusion row (seeded or
-- added later).
DROP TABLE IF EXISTS radio_play_exclusions;
