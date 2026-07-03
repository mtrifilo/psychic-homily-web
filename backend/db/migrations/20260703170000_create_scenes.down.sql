DROP TABLE IF EXISTS scenes;
-- Scene follows reference scenes.id polymorphically (no FK); left behind they
-- would re-attach to recycled BIGSERIAL ids on a re-up. Remove them with the
-- table they point into.
DELETE FROM user_bookmarks WHERE entity_type = 'scene';
