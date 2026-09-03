-- Remove the `"collection_id": 0` sentinel from collection audit metadata.
--
-- A collection audit row's metadata carries the parent collection's id so the
-- contributions timeline can decide the row against that collection. An earlier
-- writer resolved that id by a lookup that could fail and stamped the failure's
-- zero. Zero names no collection, so a row carrying it is decided by the older
-- metadata-slug arm instead, which the read gate reaches by treating the
-- sentinel as an absent key (`contributionVisibilitySQL`'s `parentIDAbsent`).
--
-- THOSE ROWS READ CORRECTLY TODAY. This is not a fix for a row being withheld;
-- it is what lets the read gate stop carrying a special case for a value no
-- writer can produce any more. Every collection mutation now returns the id of
-- the row it loaded to authorise the write, so the sentinel is bounded to rows
-- written before that. Stripping the key gives them the same shape as the rows
-- written before the key existed at all, which is the shape the slug arm was
-- built for, and `parentIDAbsent`'s `= '0'` branch can then be deleted with the
-- arm itself rather than outliving it.
--
-- WHAT THIS DOES NOT DO is backfill the real id from the recorded slug. That is
-- the change that would let the slug arm be deleted, and it is a separate
-- decision because a slug is re-takeable: resolving one today can name a
-- DIFFERENT collection than the one the row was written about. Stripping the
-- sentinel is unconditionally safe in a way that resolving it is not.
--
-- IDEMPOTENT: the predicate stops matching once the key is gone, so a re-run
-- updates zero rows.
--
-- Operator query, to count the affected rows in an environment BEFORE deploying
-- (read-only, safe on production):
--
--   SELECT count(*) FROM audit_logs
--   WHERE entity_type = 'collection'
--     AND jsonb_typeof(metadata) = 'object'
--     AND metadata->>'collection_id' = '0';
--
-- Scoped to entity_type = 'collection' because that is the only discriminator
-- whose rows the timeline reads this key from; a `collection_id` under any other
-- entity type is somebody else's field and is left alone.
--
-- The jsonb_typeof guard keeps `-` operating on an object: on a jsonb ARRAY the
-- same operator means "remove this element by value", which would rewrite a
-- payload rather than drop a key.
UPDATE audit_logs
SET metadata = metadata - 'collection_id'
WHERE entity_type = 'collection'
  AND jsonb_typeof(metadata) = 'object'
  AND metadata->>'collection_id' = '0';
