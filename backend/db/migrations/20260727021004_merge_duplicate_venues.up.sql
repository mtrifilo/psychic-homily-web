-- Merge duplicate venue records created by ingest under name variants.
--
-- Two physical rooms exist twice, each pair ingested ~2 days apart. Confirmed
-- by identical shows: same event_date to the second, same artist lineup,
-- differing only in which venue record they hang off.
--
--   106 '7th St Entry'    -> 35  '7th Street Entry'   (Minneapolis)
--   52  'Metro Gallery'   -> 113 'Metro Baltimore'    (Baltimore)
--
-- Survivors were chosen on record completeness: 35 carries the Instagram link
-- and the canonical name; 113 carries both instagram + website and 176 of the
-- 178 events in its pair.
--
-- ROOT CAUSE (not fixed here -- see the follow-up ticket): show dedup keys on
-- (artist_id, venue_id, event_date). A duplicate VENUE means a different
-- venue_id, so dedup never fires and one duplicate venue silently multiplies
-- into ~90 duplicate shows. Merging the venues without deleting those shows
-- would violate shows_artist_venue_eventdate_uniq 119 times.
--
-- Every step is existence-guarded, so this file is a clean no-op on a fresh
-- database (the CI reversibility job) and on any environment where the merge
-- already ran. golang-migrate wraps multi-statement files in a transaction, so
-- the whole merge is atomic: any constraint violation rolls the entire thing
-- back rather than leaving a half-merged venue.

-- Pairs to merge, resolved by SLUG rather than by hardcoded id.
--
-- This is a correctness guard, not a style choice: primary keys are not
-- portable across environments. In the local development seed, ids 35 and 52
-- are "Troubadour" and "First Unitarian Church" -- entirely unrelated venues.
-- A migration keyed on raw ids would silently merge the wrong rooms anywhere
-- the id space differs. Slugs are uniquely indexed (idx_venues_slug) and carry
-- the venue identity, so the joins below both resolve the pair AND act as the
-- existence guard: if either slug is absent, no row is produced and every
-- subsequent step is a clean no-op.
-- Temp tables are dropped explicitly rather than with ON COMMIT DROP: under
-- golang-migrate the whole file runs in one transaction and either works, but
-- ON COMMIT DROP makes the file unrunnable outside a transaction (psql
-- autocommits each statement, so the table disappears before the next one
-- reads it). Explicit drops keep the file replayable by hand for verification.
DROP TABLE IF EXISTS psy1581_pairs;
CREATE TEMP TABLE psy1581_pairs AS
SELECT l.id AS loser, w.id AS winner
FROM (VALUES
    ('7th-st-entry-minneapolis-mn', '7th-street-entry-minneapolis-mn'),
    ('metro-gallery-baltimore-md',  'metro-baltimore-baltimore-md')
) AS v(loser_slug, winner_slug)
JOIN venues l ON l.slug = v.loser_slug
JOIN venues w ON w.slug = v.winner_slug;

-- 1. Map each loser show to the winner show it duplicates.
--
--    "Duplicate" is defined to match shows_artist_venue_eventdate_uniq
--    exactly -- a loser show collides if any of its (artist_id, event_date)
--    pairs already exists on the winner venue. That is precisely the condition
--    that would make step 4's venue_id reassignment violate the index, so the
--    mapping and the constraint cannot drift apart.
--
--    DISTINCT ON picks one winner show deterministically when the winner venue
--    has more than one show on the same date.
DROP TABLE IF EXISTS psy1581_dup;
CREATE TEMP TABLE psy1581_dup AS
SELECT DISTINCT ON (lsa.show_id)
       lsa.show_id AS loser_show,
       wsa.show_id AS winner_show,
       pr.loser,
       pr.winner
FROM psy1581_pairs pr
JOIN show_artists lsa
  ON lsa.venue_id = pr.loser
 AND lsa.event_date IS NOT NULL
JOIN show_artists wsa
  ON wsa.artist_id  = lsa.artist_id
 AND wsa.event_date = lsa.event_date
 AND wsa.venue_id   = pr.winner
ORDER BY lsa.show_id, wsa.show_id;

-- 2. Rescue support acts before deleting anything.
--
--    A duplicate pair is not always a clean copy: 5 of the Minneapolis
--    duplicates list an artist the surviving show does not (Scaphe, Buildings,
--    and three "( solo )" / "( Live )" name variants). Deleting the loser show
--    outright would silently drop those associations, so move any
--    non-colliding artist onto the winner show first.
--
--    Both guards are load-bearing: the first keeps the move from violating
--    shows_artist_venue_eventdate_uniq, the second keeps it from violating the
--    (show_id, artist_id) primary key. Position is offset rather than reused so
--    rescued acts append after the winner's existing bill instead of competing
--    for its ordering.
UPDATE show_artists sa
SET show_id  = d.winner_show,
    venue_id = d.winner,
    position = 100 + sa.position
FROM psy1581_dup d
WHERE sa.show_id = d.loser_show
  AND NOT EXISTS (
        SELECT 1 FROM show_artists w
        WHERE w.artist_id  = sa.artist_id
          AND w.venue_id   = d.winner
          AND w.event_date = sa.event_date
      )
  AND NOT EXISTS (
        SELECT 1 FROM show_artists w2
        WHERE w2.show_id   = d.winner_show
          AND w2.artist_id = sa.artist_id
      );

-- 3. Delete the duplicate shows. show_artists and show_venues both cascade on
--    show_id, so their rows go with them.
DELETE FROM shows s
USING psy1581_dup d
WHERE s.id = d.loser_show;

-- 4. Reassign the surviving (non-duplicate) loser shows to the winner venue.
--    Drop any show_venues row that would collide on the (show_id, venue_id)
--    primary key first -- that only happens if a show was somehow attached to
--    both records.
DELETE FROM show_venues sv
USING psy1581_pairs pr
WHERE sv.venue_id = pr.loser
  AND EXISTS (
        SELECT 1 FROM show_venues w
        WHERE w.show_id = sv.show_id AND w.venue_id = pr.winner
      );

UPDATE show_venues sv
SET venue_id = pr.winner
FROM psy1581_pairs pr
WHERE sv.venue_id = pr.loser;

UPDATE show_artists sa
SET venue_id = pr.winner
FROM psy1581_pairs pr
WHERE sa.venue_id = pr.loser;

-- 5. Festivals. festival_venues is unique on (festival_id, venue_id);
--    festival_artists.venue_id is a plain nullable FK with no uniqueness.
DELETE FROM festival_venues fv
USING psy1581_pairs pr
WHERE fv.venue_id = pr.loser
  AND EXISTS (
        SELECT 1 FROM festival_venues w
        WHERE w.festival_id = fv.festival_id AND w.venue_id = pr.winner
      );

UPDATE festival_venues fv
SET venue_id = pr.winner
FROM psy1581_pairs pr
WHERE fv.venue_id = pr.loser;

UPDATE festival_artists fa
SET venue_id = pr.winner
FROM psy1581_pairs pr
WHERE fa.venue_id = pr.loser;

-- 6. notification_filters.venue_ids is a bigint[] with no FK, so a stale id
--    would linger silently. Replace, then de-duplicate in case the filter
--    already tracked the winner.
UPDATE notification_filters nf
SET venue_ids = ARRAY(
      SELECT DISTINCT x
      FROM unnest(array_replace(nf.venue_ids, pr.loser, pr.winner)) AS x
      ORDER BY x
    ),
    updated_at = NOW()
FROM psy1581_pairs pr
WHERE nf.venue_ids @> ARRAY[pr.loser]::bigint[];

-- 7. Polymorphic (entity_type, entity_id) references.
--
--    These carry no foreign key, so rows left behind would become silent
--    orphans pointing at a deleted venue rather than failing loudly. The list
--    is every table in the schema with an entity_type column that can hold
--    'venue'. For each, conflicting rows are removed before the update so the
--    unique keys listed alongside cannot be violated; tables with no relevant
--    unique key just take the update.
DO $$
DECLARE
  t         RECORD;
  keycols   text;
  join_pred text;
BEGIN
  FOR t IN
    SELECT * FROM (VALUES
        ('collection_items',       ARRAY['collection_id']),
        ('comment_last_read',      ARRAY['user_id']),
        ('comment_subscriptions',  ARRAY['user_id']),
        ('entity_tags',            ARRAY['tag_id']),
        ('image_enrich_queue',     ARRAY[]::text[]),
        ('pending_entity_edits',   ARRAY['submitted_by']),
        ('source_configs',         ARRAY[]::text[]),
        ('tag_votes',              ARRAY['tag_id', 'user_id']),
        ('user_bookmarks',         ARRAY['user_id', 'action']),
        ('audit_logs',             NULL),
        ('comments',               NULL),
        ('entity_edit_audit_logs', NULL),
        ('entity_reports',         NULL),
        ('notification_log',       NULL),
        ('revisions',              NULL)
    ) AS x(tbl, keycols)
  LOOP
    -- Skip tables absent from this database (schema drift across environments).
    CONTINUE WHEN to_regclass('public.' || t.tbl) IS NULL;

    -- NULL keycols marks an append-only / non-unique table: update only.
    IF t.keycols IS NOT NULL THEN
      SELECT string_agg(format('w.%I = l.%I', c, c), ' AND ')
        INTO join_pred
        FROM unnest(t.keycols) AS c;

      EXECUTE format($f$
        DELETE FROM %1$I l
        USING psy1581_pairs pr
        WHERE l.entity_type = 'venue'
          AND l.entity_id   = pr.loser
          AND EXISTS (
                SELECT 1 FROM %1$I w
                WHERE w.entity_type = 'venue'
                  AND w.entity_id   = pr.winner
                  %2$s
              )
      $f$, t.tbl, CASE WHEN join_pred IS NULL THEN '' ELSE 'AND ' || join_pred END);
    END IF;

    EXECUTE format($f$
      UPDATE %1$I l
      SET entity_id = pr.winner
      FROM psy1581_pairs pr
      WHERE l.entity_type = 'venue'
        AND l.entity_id   = pr.loser
    $f$, t.tbl);
  END LOOP;
END $$;

-- 8. Drop the now-unreferenced loser venues and report what moved.
DO $$
DECLARE
  n_venues bigint;
  n_shows  bigint;
BEGIN
  SELECT count(*) INTO n_shows  FROM psy1581_dup;
  SELECT count(*) INTO n_venues FROM psy1581_pairs;
  RAISE NOTICE 'PSY-1581: merged % venue pair(s), deleted % duplicate show(s)',
    n_venues, n_shows;
END $$;

DELETE FROM venues v
USING psy1581_pairs pr
WHERE v.id = pr.loser;

DROP TABLE IF EXISTS psy1581_dup;
DROP TABLE IF EXISTS psy1581_pairs;
