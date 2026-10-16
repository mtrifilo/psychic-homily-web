-- Reverse 20261015143001_seed_radio_play_exclusions_album_titles.up.sql: remove
-- the seeded global rows by their rule identity. Rows added later, and
-- station-scoped rows, are kept.
DELETE FROM radio_play_exclusions e
USING (VALUES
    ('album_title', 'exact', 'single'),
    ('album_title', 'exact', 's/t'),
    ('album_title', 'exact', 'live on kexp'),
    ('album_title', 'exact', 'live on wfmu'),
    ('album_title', 'exact', 'live at wfmu')
) AS s(match_kind, pattern_type, pattern)
WHERE e.match_kind = s.match_kind
  AND e.pattern_type = s.pattern_type
  AND e.pattern = s.pattern
  AND e.station_id IS NULL;
