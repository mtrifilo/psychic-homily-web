-- Remove the `"collection_id": 0` sentinel from collection audit metadata.
--
-- A collection audit row's metadata carries the parent collection's id so the
-- contributions timeline can decide the row against that collection. An earlier
-- writer resolved that id by a lookup that could fail, and stamped the failure's
-- zero. Zero names no collection, so such a row passes no arm of the timeline's
-- gate on the id and, with the key PRESENT, could not fall through to the older
-- slug arm either: it was withheld from everyone, including its own author on a
-- public collection.
--
-- The writers cannot produce one any more; every collection mutation now returns
-- the id of the row it loaded to authorise the write. What remains is the rows
-- written before that, and this removes the key from them so they carry the same
-- shape as the rows written before the key existed at all -- absent -- and are
-- decided by the slug arm that shape was built for.
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
