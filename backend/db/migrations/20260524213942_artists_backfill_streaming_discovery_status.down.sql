-- Reverse the backfill so the schema-drop migration sees only the column
-- default ('unreviewed'). This keeps the up→down→up round-trip clean and
-- preserves the invariant that rows touched by an admin (any non-default
-- status) aren't silently rewritten on rollback.
UPDATE artists
   SET streaming_discovery_status = 'unreviewed'
 WHERE streaming_discovery_status = 'linked'
   AND (
       spotify    IS NOT NULL
    OR bandcamp   IS NOT NULL
    OR youtube    IS NOT NULL
    OR soundcloud IS NOT NULL
   );
