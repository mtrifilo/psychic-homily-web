-- entity_former_slugs remembers the slugs a show, venue or artist no longer
-- answers at, each pointing at the entity that now stands for it: the same
-- entity after a slug change, or the survivor of a merge. The existence probe
-- reads it only after the live slug lookup has missed, so a row never changes
-- what a live slug answers, and the probe names the entity's CURRENT slug, so
-- every old URL is one redirect from the page.
--
-- It is a polymorphic (entity_type, entity_id) table with no foreign key, in
-- the shared entity-ref inventory: a merge moves the loser's rows to the
-- winner, which keeps every chain one hop long, and a delete with no successor
-- drops the deleted entity's rows. A row whose entity is gone answers nothing,
-- because the probe joins the live entity.
--
-- The triggers keep two invariants for every writer, including raw SQL and
-- the owner-run tools:
--   * a slug that changes to a different non-empty value is recorded against
--     the entity;
--   * a slug a live row takes (by insert or by update, including an entity
--     taking back one of its own former slugs) is removed from the table, so
--     a former slug can never shadow or outlive a live claim.
--
-- Creating the triggers takes a SHARE ROW EXCLUSIVE lock on shows, venues and
-- artists, which waits for writers; lock_timeout bounds that wait.
SET LOCAL lock_timeout = '5s';

CREATE TABLE entity_former_slugs (
    entity_type VARCHAR(20)  NOT NULL,
    former_slug VARCHAR(255) NOT NULL,
    entity_id   INT          NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (entity_type, former_slug),
    CONSTRAINT entity_former_slugs_entity_type_check
        CHECK (entity_type IN ('show', 'venue', 'artist')),
    CONSTRAINT entity_former_slugs_former_slug_check
        CHECK (former_slug <> '')
);

-- The merge re-point and the delete sweep find an entity's rows by id.
CREATE INDEX entity_former_slugs_entity_idx ON entity_former_slugs (entity_type, entity_id);

-- TG_ARGV[0] is the row's entity_type value.
--
-- The unique slug index means a row can take a slug in the same statement that
-- frees it only after the row that held it has changed; AFTER row triggers fire
-- in the order the statement changed its rows, so the taker's trigger always
-- runs after the recording one and removes what it recorded.
CREATE FUNCTION entity_former_slugs_track()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.slug IS NOT NULL AND OLD.slug <> '' THEN
        INSERT INTO entity_former_slugs (entity_type, former_slug, entity_id)
        VALUES (TG_ARGV[0], OLD.slug, NEW.id)
        ON CONFLICT (entity_type, former_slug)
        DO UPDATE SET entity_id = EXCLUDED.entity_id, created_at = now();
    END IF;

    DELETE FROM entity_former_slugs
     WHERE entity_type = TG_ARGV[0]
       AND former_slug = NEW.slug;

    RETURN NULL;
END;
$$;

CREATE TRIGGER shows_track_former_slug_on_update
AFTER UPDATE OF slug ON shows
FOR EACH ROW
WHEN (OLD.slug IS DISTINCT FROM NEW.slug AND NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('show');

CREATE TRIGGER shows_track_former_slug_on_insert
AFTER INSERT ON shows
FOR EACH ROW
WHEN (NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('show');

CREATE TRIGGER venues_track_former_slug_on_update
AFTER UPDATE OF slug ON venues
FOR EACH ROW
WHEN (OLD.slug IS DISTINCT FROM NEW.slug AND NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('venue');

CREATE TRIGGER venues_track_former_slug_on_insert
AFTER INSERT ON venues
FOR EACH ROW
WHEN (NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('venue');

CREATE TRIGGER artists_track_former_slug_on_update
AFTER UPDATE OF slug ON artists
FOR EACH ROW
WHEN (OLD.slug IS DISTINCT FROM NEW.slug AND NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('artist');

CREATE TRIGGER artists_track_former_slug_on_insert
AFTER INSERT ON artists
FOR EACH ROW
WHEN (NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('artist');

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
