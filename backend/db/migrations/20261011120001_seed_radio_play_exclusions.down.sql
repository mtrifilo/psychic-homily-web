-- Reverse 20261011120001_seed_radio_play_exclusions.up.sql: remove the seeded
-- global rows by their rule identity. Rows added later, and station-scoped rows,
-- are kept.
DELETE FROM radio_play_exclusions e
USING (VALUES
    ('artist_name', 'regex', '^music behind dj(:|$)'),
    ('artist_name', 'regex', '^.+ speaks$'),
    ('artist_name', 'exact', 'mic break'),
    ('artist_name', 'exact', 'clip'),
    ('artist_name', 'exact', 'tv clip'),
    ('artist_name', 'exact', 'shake some action recap'),
    ('artist_name', 'exact', 'where the action is! with dr. jeff'),
    ('artist_name', 'exact', 'wake weather'),
    ('artist_name', 'exact', 'unknown artist'),
    ('artist_name', 'exact', 'unknown artists'),
    ('artist_name', 'exact', 'unknown'),
    ('artist_name', 'exact', 'various artists'),
    ('artist_name', 'exact', 'various'),
    ('artist_name', 'exact', 'va'),
    ('artist_name', 'exact', 'v/a'),
    ('artist_name', 'exact', 'n/a'),
    ('artist_name', 'exact', 'no artist'),
    ('track_title', 'regex', '^your dj speaks'),
    ('track_title', 'exact', 'we are on a break'),
    ('track_title', 'exact', 'unknown track'),
    ('label_name', 'regex', '^no label( |:|$)'),
    ('label_name', 'regex', '^self[- ]*releas'),
    ('label_name', 'exact', 'self'),
    ('label_name', 'regex', '^not on (a )?label'),
    ('label_name', 'exact', 'n/a'),
    ('label_name', 'exact', 'none'),
    ('label_name', 'exact', 'unknown'),
    ('label_name', 'exact', 'unreleased'),
    ('label_name', 'exact', 'bandcamp'),
    ('label_name', 'exact', 'white label'),
    ('label_name', 'exact', 'bootleg')
) AS s(match_kind, pattern_type, pattern)
WHERE e.match_kind = s.match_kind
  AND e.pattern_type = s.pattern_type
  AND e.pattern = s.pattern
  AND e.station_id IS NULL;
