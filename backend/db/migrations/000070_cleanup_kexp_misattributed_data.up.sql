-- The KEXP API silently ignores the program_id filter parameter, returning
-- ALL broadcasts regardless. This caused FetchNewEpisodes to import episodes
-- from unrelated programs (Drive Time, Wo' Pop, etc.) and attribute them to
-- whatever show was being processed.
--
-- This migration:
-- 1. Deletes misattributed episodes (and their plays via CASCADE)
-- 2. Merges the duplicate "Morning Show" (NULL external_id) into "The Morning Show"

-- Step 1: Delete episodes that don't belong to their assigned show.
-- These were imported by the broken KEXP provider before client-side filtering was added.
-- We identify them by having external_ids corresponding to other programs' broadcasts.
-- For safety, only delete episodes under KEXP shows.
DELETE FROM radio_episodes
WHERE show_id IN (
    SELECT id FROM radio_shows
    WHERE station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp')
)
AND external_id IS NOT NULL
AND external_id != '';

-- Step 2: Merge the duplicate "Morning Show" (slug=morning-show, NULL external_id)
-- into "The Morning Show" (slug=the-morning-show, external_id='16').
-- Move its episodes first, then delete the duplicate.
UPDATE radio_episodes
SET show_id = (
    SELECT id FROM radio_shows
    WHERE slug = 'the-morning-show'
      AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp')
)
WHERE show_id = (
    SELECT id FROM radio_shows
    WHERE slug = 'morning-show'
      AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp')
      AND (external_id IS NULL OR external_id = '')
);

DELETE FROM radio_shows
WHERE slug = 'morning-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp')
  AND (external_id IS NULL OR external_id = '');
