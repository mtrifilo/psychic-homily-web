-- PSY-576: backfill the denorm columns added by the previous migration.
-- Uses a LATERAL subquery so a show with multiple show_venues rows picks
-- the lowest venue_id deterministically (PSY-628 spike doc). Local seed
-- has 0 multi-venue shows but prod may.
UPDATE show_artists sa
SET event_date = s.event_date,
    venue_id   = pv.venue_id
FROM shows s
JOIN LATERAL (
    SELECT venue_id
    FROM show_venues
    WHERE show_id = s.id
    ORDER BY venue_id
    LIMIT 1
) pv ON TRUE
WHERE sa.show_id = s.id;
