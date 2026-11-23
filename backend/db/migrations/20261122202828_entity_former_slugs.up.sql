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
-- The triggers that keep it, one table per migration so that no transaction
-- holds more than one of those tables' locks, are in 20261122202829 (venues),
-- 20261122202830 (artists) and 20261122202831 (shows); the backfill is
-- 20261122202832. This file creates only the new table and the trigger
-- function, which lock nothing another session uses; lock_timeout bounds the
-- wait for the locks they do take.
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

-- The function the per-table triggers run. They keep two invariants for every
-- writer, including raw SQL and the owner-run tools:
--   * a slug that changes to a different non-empty value is recorded against
--     the entity;
--   * a slug a live row takes (by insert or by update, including an entity
--     taking back one of its own former slugs) is removed from the table, so
--     a former slug can never shadow or outlive a live claim.
--
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
