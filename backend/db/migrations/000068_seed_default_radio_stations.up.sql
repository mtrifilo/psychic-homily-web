-- Seed the three radio stations with dedicated provider code (KEXP, WFMU, NTS)
-- and their flagship shows so they exist without running the seed command.

INSERT INTO radio_stations (name, slug, description, city, state, country, timezone, stream_url, website, donation_url, broadcast_type, frequency_mhz, playlist_source, is_active)
VALUES
    ('KEXP', 'kexp', 'KEXP is a listener-supported, non-commercial radio station in Seattle, Washington, known for championing independent and emerging artists across all genres.', 'Seattle', 'WA', 'US', 'America/Los_Angeles', 'https://kexp.streamguys1.com/kexp160.aac', 'https://www.kexp.org', 'https://www.kexp.org/donate/', 'both', 90.3, 'kexp_api', TRUE),
    ('WFMU', 'wfmu', 'WFMU is the longest-running freeform radio station in the United States, broadcasting from Jersey City, New Jersey. Known for its eclectic, unconstrained programming.', 'Jersey City', 'NJ', 'US', 'America/New_York', 'https://stream0.wfmu.org/freeform-128k', 'https://wfmu.org', 'https://wfmu.org/marathon/', 'both', 91.1, 'wfmu_scrape', TRUE),
    ('NTS Radio', 'nts-radio', 'NTS is an online radio station based in London, broadcasting 24/7 across two channels with shows from over 500 residents worldwide.', 'London', NULL, 'GB', 'Europe/London', 'https://stream-relay-geo.ntslive.net/stream', 'https://www.nts.live', 'https://www.nts.live/supporters', 'internet', NULL, 'nts_api', TRUE)
ON CONFLICT (slug) DO NOTHING;

-- KEXP shows
INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'kexp'), 'The Morning Show', 'the-morning-show', 'John Richards', 'KEXP''s flagship morning program featuring a hand-picked mix of new and classic tracks.', 'Weekdays 6-10 AM PT', 'https://www.kexp.org/shows/the-morning-show/', '1', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'kexp'), 'The Midday Show', 'the-midday-show', 'Cheryl Waters', 'A mid-day mix of new music discoveries and deep cuts.', 'Weekdays 10 AM-2 PM PT', 'https://www.kexp.org/shows/the-midday-show/', '2', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'kexp'), 'The Afternoon Show', 'the-afternoon-show', 'Kevin Cole', 'KEXP afternoon programming with a mix of established and emerging artists.', 'Weekdays 2-6 PM PT', 'https://www.kexp.org/shows/the-afternoon-show/', '3', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'kexp'), 'Audioasis', 'audioasis', 'Kennady Quille', 'KEXP''s long-running Northwest music show spotlighting artists from the Pacific Northwest.', 'Saturdays 6-9 PM PT', 'https://www.kexp.org/shows/Audioasis/', '4', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'kexp'), 'El Sonido', 'el-sonido', 'Albina Cabrera, Goyri', 'A trip around the diverse world of Latin music and culture.', 'Saturdays 9 PM-12 AM PT', 'https://www.kexp.org/shows/El-Sonido/', '5', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'kexp'), 'Midnight in a Perfect World', 'midnight-in-a-perfect-world', NULL, 'Late-night electronic music exploring ambient, house, techno, and experimental sounds.', 'Saturdays 12-3 AM PT', 'https://www.kexp.org/shows/Midnight-in-a-Perfect-World/', '6', TRUE)
ON CONFLICT (slug) DO NOTHING;

-- WFMU shows
INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'wfmu'), 'Trouble', 'trouble-wfmu', 'Doug Schulkind', 'Eclectic freeform with an emphasis on soul, jazz, international, and the unexpected.', 'Wednesdays 10 AM-1 PM ET', 'https://wfmu.org/playlists/DS', 'DS', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'wfmu'), 'The Best Show', 'the-best-show-wfmu', 'Tom Scharpling', 'Comedy and music variety show, one of the longest-running call-in shows in radio history.', 'Tuesdays 9 PM-12 AM ET', 'https://wfmu.org/playlists/TS', 'TS', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'wfmu'), 'Bodega Pop Live', 'bodega-pop-live-wfmu', 'Mike Lupica', 'A window into the sounds of immigrant communities in New York City and beyond.', 'Saturdays 10 AM-1 PM ET', 'https://wfmu.org/playlists/MX', 'MX', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'wfmu'), 'Downtown Soulville', 'downtown-soulville-wfmu', 'Mr. Fine Wine', 'Deep soul, Northern soul, sweet soul, and classic R&B from the 1950s through 1970s.', 'Saturdays 6-9 PM ET', 'https://wfmu.org/playlists/FW', 'FW', TRUE)
ON CONFLICT (slug) DO NOTHING;

-- NTS Radio shows
INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'nts-radio'), 'Floating Points', 'floating-points-nts', 'Floating Points', 'Eclectic selections spanning jazz, electronic, ambient, and world music from producer Floating Points.', 'Monthly', 'https://www.nts.live/shows/floating-points', 'floating-points', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'nts-radio'), 'Charlie Bones', 'charlie-bones-nts', 'Charlie Bones', 'An eclectic morning show blending jazz, soul, funk, and left-field selections.', 'Weekdays 10 AM-1 PM GMT', 'https://www.nts.live/shows/charlie-bones', 'charlie-bones', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'nts-radio'), 'Brownswood Basement', 'brownswood-basement-nts', 'Gilles Peterson', 'Gilles Peterson digs deep into jazz, beats, soul, and global sounds.', 'Bi-weekly', 'https://www.nts.live/shows/brownswood-basement', 'brownswood-basement', TRUE),
    ((SELECT id FROM radio_stations WHERE slug = 'nts-radio'), 'Anu', 'anu-nts', 'Anu', 'A mix of left-field club, electronic, and experimental sounds.', 'Monthly', 'https://www.nts.live/shows/anu', 'anu', TRUE)
ON CONFLICT (slug) DO NOTHING;
