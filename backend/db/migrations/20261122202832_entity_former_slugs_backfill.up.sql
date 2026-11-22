-- Backfills entity_former_slugs with the slugs a record kept exactly. It
-- reads venues, artists and audit_logs and writes only entity_former_slugs,
-- so it takes no lock on shows, venues or artists that blocks their writers;
-- lock_timeout bounds the wait for the locks it does take.
--
-- A recorded value is written only when it has the shape of a slug the
-- application generates: lowercase letters, digits and hyphens, starting and
-- ending with a letter or digit, at most 255 characters. Anything else in an
-- audit row (a placeholder for an entity with no slug, free text) is skipped.
SET LOCAL lock_timeout = '5s';

-- The two venue merges migration 20260727021004 performed name their merged
-- slugs exactly. Each row is written only where its survivor resolves by slug
-- and the merged slug is free, so a database where either side is absent (a
-- fresh one, a seed, an environment where that merge never ran) gets nothing.
INSERT INTO entity_former_slugs (entity_type, former_slug, entity_id)
SELECT 'venue', pair.former_slug, survivor.id
FROM (VALUES
    ('7th-st-entry-minneapolis-mn', '7th-street-entry-minneapolis-mn'),
    ('metro-gallery-baltimore-md',  'metro-baltimore-baltimore-md')
) AS pair(former_slug, survivor_slug)
JOIN venues survivor ON survivor.slug = pair.survivor_slug
WHERE NOT EXISTS (SELECT 1 FROM venues live WHERE live.slug = pair.former_slug)
ON CONFLICT DO NOTHING;

-- A venue merge records the merged room's slug in its audit row
-- (merged_venue_slug), and the artist triage tool records the merged artist's
-- (merged_artist_slug). An audit row's entity_id follows later merges to the
-- current survivor. Each recorded slug is written against a survivor that
-- still exists, only while no live row holds it, and the latest merge of a
-- slug wins.
INSERT INTO entity_former_slugs (entity_type, former_slug, entity_id)
SELECT DISTINCT ON (recorded.entity_type, recorded.former_slug)
       recorded.entity_type, recorded.former_slug, recorded.survivor_id
FROM (
    SELECT 'venue' AS entity_type, a.metadata->>'merged_venue_slug' AS former_slug,
           a.entity_id AS survivor_id, a.created_at, a.id
    FROM audit_logs a
    JOIN venues survivor ON survivor.id = a.entity_id
    WHERE a.action = 'merge_venues' AND a.entity_type = 'venue'
    UNION ALL
    SELECT 'artist', a.metadata->>'merged_artist_slug', a.entity_id, a.created_at, a.id
    FROM audit_logs a
    JOIN artists survivor ON survivor.id = a.entity_id
    WHERE a.action = 'merge_artists' AND a.entity_type = 'artist'
) recorded
WHERE recorded.former_slug ~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
  AND length(recorded.former_slug) <= 255
  AND NOT (recorded.entity_type = 'venue'
           AND EXISTS (SELECT 1 FROM venues live WHERE live.slug = recorded.former_slug))
  AND NOT (recorded.entity_type = 'artist'
           AND EXISTS (SELECT 1 FROM artists live WHERE live.slug = recorded.former_slug))
ORDER BY recorded.entity_type, recorded.former_slug, recorded.created_at DESC, recorded.id DESC
ON CONFLICT DO NOTHING;
