-- Flip existing artists with any music-platform link to 'linked' so the
-- worklist doesn't surface already-curated rows on first run.
--
-- "Music-platform link" = spotify | bandcamp | youtube | soundcloud.
-- The other Social fields (instagram/facebook/twitter/website) aren't
-- music-platform-specific — a band's Instagram doesn't constitute a
-- streaming-discovery link.
UPDATE artists
   SET streaming_discovery_status = 'linked'
 WHERE streaming_discovery_status = 'unreviewed'
   AND (
       spotify    IS NOT NULL
    OR bandcamp   IS NOT NULL
    OR youtube    IS NOT NULL
    OR soundcloud IS NOT NULL
   );
