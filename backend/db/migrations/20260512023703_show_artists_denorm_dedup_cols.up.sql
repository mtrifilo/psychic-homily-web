-- PSY-576: denormalize event_date + venue_id onto show_artists so a single
-- partial unique index can structurally enforce the show dedup key
-- (artist_id, venue_id, event_date). Columns are nullable here; the
-- backfill DML lands in the next migration, and the partial unique index
-- excludes NULL so any future bulk-insert path that forgets to populate
-- these degrades gracefully (the row inserts, the index just skips it).
ALTER TABLE show_artists
  ADD COLUMN event_date TIMESTAMPTZ,
  ADD COLUMN venue_id   INTEGER REFERENCES venues(id) ON DELETE CASCADE;
