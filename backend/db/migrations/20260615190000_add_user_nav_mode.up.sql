-- PSY-1115: per-account global nav-style preference (top-bar vs left side nav).
-- Additive, NOT NULL with a 'top' default so existing rows are backfilled to
-- the current top-bar nav; the CHECK keeps the column to the two known modes
-- (mirrors models/auth.NavModeTop / NavModeSide and the frontend encoding).
-- Adding a column with a constant default is a metadata-only change in
-- Postgres 11+, so no table rewrite / CONCURRENTLY concern.
ALTER TABLE users
  ADD COLUMN nav_mode VARCHAR(8) NOT NULL DEFAULT 'top'
  CHECK (nav_mode IN ('top', 'side'));
