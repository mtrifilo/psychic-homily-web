-- Add the two show-time columns the show page renders alongside the date:
-- doors_at (when the room opens) and music_at (when the first set starts).
--
-- Both are nullable and independent of event_date. event_date stays the
-- canonical "when is this show" instant that every query, slug, dedup index,
-- and JSON-LD startDate reads; these two are display detail that is often
-- unknown and must never become a second source of truth for the show's date.
--
-- TIMESTAMPTZ to match event_date (converted in 000028) so all three compare
-- and render through the same venue-timezone path. No end-time column: the
-- show page derives an end estimate from doors and labels it as an estimate,
-- and a guess must not be persisted or published as fact.
ALTER TABLE shows
    ADD COLUMN doors_at TIMESTAMPTZ,
    ADD COLUMN music_at TIMESTAMPTZ;
