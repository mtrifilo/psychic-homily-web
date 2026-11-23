-- Undoes 20261122202832 only: removes the rows its backfill could have
-- written, the two migration-merged venue slugs and every slug a venue or
-- artist merge's audit row names. A merge run after the backfill records the
-- same slug the same way, so such a row is removed with them.
SET LOCAL lock_timeout = '5s';

DELETE FROM entity_former_slugs
 WHERE entity_type = 'venue'
   AND former_slug IN ('7th-st-entry-minneapolis-mn', 'metro-gallery-baltimore-md');

DELETE FROM entity_former_slugs f
 USING audit_logs a
 WHERE (f.entity_type = 'venue' AND a.action = 'merge_venues' AND a.entity_type = 'venue'
        AND f.former_slug = a.metadata->>'merged_venue_slug')
    OR (f.entity_type = 'artist' AND a.action = 'merge_artists' AND a.entity_type = 'artist'
        AND f.former_slug = a.metadata->>'merged_artist_slug');
