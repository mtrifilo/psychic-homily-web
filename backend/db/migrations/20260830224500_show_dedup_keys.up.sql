-- show_dedup_keys is the dedup key of a show, materialized at the grain the key
-- actually has: one row per (show, artist, venue) with the show's event_date.
--
-- shows_artist_venue_eventdate_uniq lives on show_artists, whose primary key is
-- (show_id, artist_id). A show billed at two venues therefore has only ONE row
-- per artist to stamp a venue_id onto, so the stamping picks the lowest venue id
-- and the index covers that venue alone. Every other venue of a multi-venue bill
-- is unprotected: a second show at one of them, billing the same artist on the
-- same timestamp, inserts cleanly. The name-keyed application guard hides that
-- today because it matches on venue and artist NAMES, but it only inspects
-- headliners, so a bill's curated opener walks straight past it.
--
-- A relational UNIQUE cannot span the shows -> show_venues -> show_artists join,
-- so the join is materialized here and the constraint put on the result. The
-- table is derived, never authored: triggers rebuild a show's rows from the
-- association tables on every write, which is what makes the key hold for
-- callers that never go through the service layer.
CREATE TABLE show_dedup_keys (
    show_id    INT NOT NULL REFERENCES shows(id)   ON DELETE CASCADE,
    artist_id  INT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    venue_id   INT NOT NULL REFERENCES venues(id)  ON DELETE CASCADE,
    event_date TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (show_id, artist_id, venue_id),
    CONSTRAINT show_dedup_keys_artist_venue_date_uniq UNIQUE (artist_id, venue_id, event_date)
);

-- The venue merge scans every key at a venue with no artist and no show id to
-- narrow it, which neither the primary key nor the unique constraint can serve:
-- both lead with a column that scan does not know. Lookups keyed on the artist
-- already ride the unique constraint's own index.
CREATE INDEX show_dedup_keys_venue_date_idx ON show_dedup_keys (venue_id, event_date);

-- Rebuilds one show's rows from the association tables. DELETE first so a
-- removed artist or venue drops out, then upsert so a moved event_date follows
-- without disturbing rows that did not change.
--
-- The whole cross product is rebuilt rather than the single row a trigger saw,
-- because a venue write changes every artist's key and an artist write changes
-- every venue's. One statement pair that reads the final state is both cheaper
-- to reason about and immune to the order rows arrive in.
CREATE OR REPLACE FUNCTION show_dedup_keys_rebuild(p_show_id INT)
RETURNS void
LANGUAGE sql
AS $$
    DELETE FROM show_dedup_keys d
    WHERE d.show_id = p_show_id
      AND NOT EXISTS (
            SELECT 1
            FROM show_artists sa
            JOIN show_venues sv ON sv.show_id = sa.show_id
            WHERE sa.show_id   = p_show_id
              AND sa.artist_id = d.artist_id
              AND sv.venue_id  = d.venue_id
          );

    INSERT INTO show_dedup_keys (show_id, artist_id, venue_id, event_date)
    SELECT sa.show_id, sa.artist_id, sv.venue_id, s.event_date
    FROM show_artists sa
    JOIN show_venues sv ON sv.show_id = sa.show_id
    JOIN shows s        ON s.id       = sa.show_id
    WHERE sa.show_id = p_show_id
    ON CONFLICT (show_id, artist_id, venue_id)
    DO UPDATE SET event_date = EXCLUDED.event_date;
$$;

-- show_artists and show_venues both carry show_id, and an UPDATE can move a row
-- between shows, so both the row's old and new show are rebuilt.
CREATE OR REPLACE FUNCTION show_dedup_keys_sync_link()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP <> 'INSERT' THEN
        PERFORM show_dedup_keys_rebuild(OLD.show_id);
    END IF;
    IF TG_OP <> 'DELETE' AND (TG_OP = 'INSERT' OR NEW.show_id <> OLD.show_id) THEN
        PERFORM show_dedup_keys_rebuild(NEW.show_id);
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION show_dedup_keys_sync_show()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM show_dedup_keys_rebuild(NEW.id);
    RETURN NULL;
END;
$$;

-- AFTER row triggers are queued and fired once the whole statement has been
-- applied, so each rebuild reads the statement's final state rather than a
-- half-updated one. A set-based re-point therefore cannot trip the constraint on
-- an intermediate row order.
CREATE TRIGGER show_artists_sync_dedup_keys
AFTER INSERT OR UPDATE OF show_id, artist_id OR DELETE ON show_artists
FOR EACH ROW EXECUTE FUNCTION show_dedup_keys_sync_link();

CREATE TRIGGER show_venues_sync_dedup_keys
AFTER INSERT OR UPDATE OF show_id, venue_id OR DELETE ON show_venues
FOR EACH ROW EXECUTE FUNCTION show_dedup_keys_sync_link();

-- shows only matters when the date moves; DELETE is covered by the cascade.
CREATE TRIGGER shows_sync_dedup_keys
AFTER UPDATE OF event_date ON shows
FOR EACH ROW
WHEN (OLD.event_date IS DISTINCT FROM NEW.event_date)
EXECUTE FUNCTION show_dedup_keys_sync_show();

-- Backfill.
--
-- ON CONFLICT DO NOTHING is the disposition for rows that already violate the
-- new key, and the ordering offers the earliest-created show first, matching the
-- winner MergeDuplicateShow keeps. Such a pair is a real duplicate that predates
-- this constraint: collapsing it needs the reference re-points cmd/dedup-shows
-- performs and cannot be done from a schema migration, so both shows are kept
-- and the later one simply has no key row until the CLI collapses it. Failing
-- the migration instead would take the deploy down over data the deploy did not
-- create.
--
-- RAISE WARNING rather than NOTICE because NOTICE sits below the default
-- log_min_messages and the migrate client does not forward either, so a NOTICE
-- would be a record nobody ever reads. This is still only a hint: the durable
-- enumeration is `cmd/dedup-shows --dry-run`, which finds the same pairs from
-- the association tables and is what actually collapses them.
INSERT INTO show_dedup_keys (show_id, artist_id, venue_id, event_date)
SELECT sa.show_id, sa.artist_id, sv.venue_id, s.event_date
FROM show_artists sa
JOIN show_venues sv ON sv.show_id = sa.show_id
JOIN shows s        ON s.id       = sa.show_id
ORDER BY s.created_at, s.id
ON CONFLICT DO NOTHING;

DO $$
DECLARE
    skipped BIGINT;
BEGIN
    SELECT count(*) INTO skipped
    FROM (
        SELECT sa.show_id, sa.artist_id, sv.venue_id
        FROM show_artists sa
        JOIN show_venues sv ON sv.show_id = sa.show_id
        EXCEPT
        SELECT show_id, artist_id, venue_id FROM show_dedup_keys
    ) missing;

    IF skipped > 0 THEN
        RAISE WARNING 'show_dedup_keys backfill skipped % pre-existing duplicate billing(s); run cmd/dedup-shows to collapse them', skipped;
    END IF;
END;
$$;
