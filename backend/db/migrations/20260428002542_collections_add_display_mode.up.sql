-- Add display_mode to collections to support ranked vs. unranked rendering.
-- PSY-348: ranked mode renders numbered positions with drag-and-drop reorder;
-- unranked mode renders a flat grid without numbers.
--
-- NOT NULL with DEFAULT 'unranked' so existing rows backfill automatically and
-- the frontend never has to handle NULL. Creators opt in to 'ranked' via the
-- collection edit UI. The CHECK constraint guards the enum at the DB level so
-- a buggy client can't write an invalid value.
ALTER TABLE collections
    ADD COLUMN display_mode VARCHAR(16) NOT NULL DEFAULT 'unranked'
        CHECK (display_mode IN ('ranked', 'unranked'));
