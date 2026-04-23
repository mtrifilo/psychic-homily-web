-- PSY-402 + PSY-408: repair NTS and WFMU radio show seed data.
-- Migration 000068 seeded several fabricated host/show/DJ-code combinations.
-- Research on 2026-04-22 against nts.live and wfmu.org established the
-- correct mappings; findings are summarized in PSY-408 comments.

-- WFMU repairs ------------------------------------------------------------
-- DS is Doug Schulkind's correct DJ code, but his current show is
-- "Give The Drummer Some" — not "Trouble" (Trouble is a different WFMU DJ
-- whose code is LM). Rename in place; archive_url stays valid.
UPDATE radio_shows
SET name = 'Give The Drummer Some',
    slug = 'give-the-drummer-some-wfmu',
    description = 'Freeform radio spanning jazz, soul, gospel, country, and global grooves, from longtime WFMU DJ Doug Schulkind.',
    schedule_display = 'Fridays 9 AM-Noon ET',
    updated_at = NOW()
WHERE slug = 'trouble-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

-- "Bodega Pop Live" was fabricated. WFMU's real show is "Bodega Pop",
-- hosted by Gary Sullivan (not Mike Lupica), DJ code PG (not MX).
UPDATE radio_shows
SET name = 'Bodega Pop',
    slug = 'bodega-pop-wfmu',
    host_name = 'Gary Sullivan',
    description = 'Global pop, regional hits, and bodega-aisle rarities.',
    schedule_display = 'Wednesdays (weekly)',
    archive_url = 'https://wfmu.org/playlists/PG',
    external_id = 'PG',
    updated_at = NOW()
WHERE slug = 'bodega-pop-live-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

-- Downtown Soulville / Mr. Fine Wine: correct host and show name, but the
-- DJ code is SV — the original "FW" belongs to F. Windhausen.
UPDATE radio_shows
SET archive_url = 'https://wfmu.org/playlists/SV',
    external_id = 'SV',
    updated_at = NOW()
WHERE slug = 'downtown-soulville-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

-- Tom Scharpling's "The Best Show" left WFMU in December 2013. The "TS"
-- DJ code now belongs to an unrelated show. No active replacement.
DELETE FROM radio_shows
WHERE slug = 'the-best-show-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

-- NTS repairs -------------------------------------------------------------
-- Charlie Bones hosts "The Do!! You!!! Breakfast Show" on NTS; the prior
-- /shows/charlie-bones URL 404s. Update name + URL + external_id to match
-- the canonical show slug.
UPDATE radio_shows
SET name = 'The Do!! You!!! Breakfast Show w/ Charlie Bones',
    archive_url = 'https://www.nts.live/shows/the-do-you-breakfast-show',
    external_id = 'the-do-you-breakfast-show',
    updated_at = NOW()
WHERE slug = 'charlie-bones-nts'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'nts-radio');

-- Brownswood Basement / Gilles Peterson has not had a resident NTS slot
-- since 2017. There is no current show to point to — remove from seed.
DELETE FROM radio_shows
WHERE slug = 'brownswood-basement-nts'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'nts-radio');
