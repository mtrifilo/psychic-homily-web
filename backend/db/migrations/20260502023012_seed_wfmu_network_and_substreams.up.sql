-- PSY-508: Seed the WFMU network row, backfill the existing wfmu station's
-- network_id, and insert the 3 stream-only sub-stations.
--
-- WFMU is a flagship 91.1 FM broadcast plus three 24/7 stream-only
-- sub-channels (Give the Drummer Radio / Sheena's Jungle Room / Rock'n'Soul
-- Radio). All four are siblings under the same network — no parent_station_id
-- column, no hierarchy. The 3 sub-streams have no programmed schedule, so
-- they're seeded without radio_shows. The background fetch service is a
-- per-show loop; with no shows associated, it's a no-op for these stations
-- and won't error.

INSERT INTO radio_networks (slug, name)
VALUES ('wfmu', 'WFMU')
ON CONFLICT (slug) DO NOTHING;

UPDATE radio_stations
   SET network_id = (SELECT id FROM radio_networks WHERE slug = 'wfmu')
 WHERE slug = 'wfmu';

INSERT INTO radio_stations (
    name, slug, description, city, state, country, timezone, stream_url,
    website, broadcast_type, playlist_source, network_id, is_active
) VALUES
    (
        'Give the Drummer Radio',
        'wfmu-drummer',
        'WFMU stream-only 24/7 channel curated by Doug Schulkind. Eclectic blends of soul, jazz, gospel, country, and global grooves.',
        'Jersey City', 'NJ', 'US', 'America/New_York',
        'https://wfmu.org/drummer',
        'https://wfmu.org/drummer',
        'internet',
        'wfmu_scrape',
        (SELECT id FROM radio_networks WHERE slug = 'wfmu'),
        TRUE
    ),
    (
        'Rock''n''Soul Radio',
        'wfmu-rocknsoulradio',
        'WFMU stream-only 24/7 channel programming rock and soul.',
        'Jersey City', 'NJ', 'US', 'America/New_York',
        'https://wfmu.org/rocknsoulradio',
        'https://wfmu.org/rocknsoulradio',
        'internet',
        'wfmu_scrape',
        (SELECT id FROM radio_networks WHERE slug = 'wfmu'),
        TRUE
    ),
    (
        'Sheena''s Jungle Room',
        'wfmu-sheena',
        'WFMU stream-only 24/7 channel.',
        'Jersey City', 'NJ', 'US', 'America/New_York',
        'https://wfmu.org/sheena',
        'https://wfmu.org/sheena',
        'internet',
        'wfmu_scrape',
        (SELECT id FROM radio_networks WHERE slug = 'wfmu'),
        TRUE
    )
ON CONFLICT (slug) DO NOTHING;
