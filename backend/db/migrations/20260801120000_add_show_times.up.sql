-- Add the two show-time columns the show page will render alongside the date:
-- doors_at (when the room opens) and music_at (when the first set starts).
-- They are readable and writable on the show create/update/read API from this
-- change onward; no UI renders them yet.
--
-- Both are nullable and independent of event_date. event_date stays the
-- canonical "when is this show" instant that every query, slug, dedup index,
-- and JSON-LD startDate reads; these two are display detail that is often
-- unknown and must never become a second source of truth for the show's date.
--
-- TIMESTAMPTZ to match event_date (converted in 000028) so all three compare
-- and render through the same venue-timezone path. No end-time column: the
-- end time is to be derived at render time and labelled an estimate, and a
-- guess must not be persisted or published as fact.
ALTER TABLE shows
    ADD COLUMN doors_at TIMESTAMPTZ,
    ADD COLUMN music_at TIMESTAMPTZ;
