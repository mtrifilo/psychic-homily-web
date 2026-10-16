-- Seed the album_title placeholder rows: album names that do not name a
-- release, so they never count as release evidence. Every row is global
-- (station_id NULL), exact, and stored in radio_normalize_name form. Each
-- reason carries the stage measurement (2026-10-14, all plays) that put the
-- name here.

INSERT INTO radio_play_exclusions (match_kind, pattern_type, pattern, category, reason) VALUES
    ('album_title', 'exact', 'single', 'placeholder',
     'Release-format placeholder, not an album title. Stage: 3338 plays under 2441 artist names.'),
    ('album_title', 'exact', 's/t', 'placeholder',
     'Self-titled placeholder; it names no album title of its own. Stage: 259 plays under 210 artist names.'),
    ('album_title', 'exact', 'live on kexp', 'placeholder',
     'KEXP session name logged as the album, one title across many artists'' sessions. Stage: 641 plays under 156 artist names.'),
    ('album_title', 'exact', 'live on wfmu', 'placeholder',
     'WFMU session name logged as the album. Stage: 151 plays under 21 artist names.'),
    ('album_title', 'exact', 'live at wfmu', 'placeholder',
     'WFMU session name logged as the album. Stage: 57 plays under 9 artist names.')
ON CONFLICT ON CONSTRAINT radio_play_exclusions_rule_unique DO NOTHING;
