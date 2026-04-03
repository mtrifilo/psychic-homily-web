-- Fix CLI-submitted shows (IDs 667-687) that stored local times as UTC.
-- These were submitted via the CLI which appended 'Z' without converting
-- from the venue's local timezone. Each show needs its event_date shifted
-- forward by the venue's UTC offset to get the correct UTC time.
--
-- The shift is determined by each venue's state timezone:
--   Arizona (America/Phoenix, UTC-7, no DST): +7 hours
--   Other states would use their respective offsets, but all 21 shows
--   in this batch are at Arizona venues.

-- Shift all 21 shows by +7 hours (Arizona, UTC-7, no DST)
-- This converts "2026-04-15 20:00:00 UTC" (which was really 8pm MST)
-- to "2026-04-16 03:00:00 UTC" (correct UTC for 8pm MST)
UPDATE shows
SET event_date = event_date + INTERVAL '7 hours',
    updated_at = NOW()
WHERE id BETWEEN 667 AND 687;
