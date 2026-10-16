-- Reverse 20261015210000_radio_plays_release_link_source.up.sql. Dropping the
-- column drops its CHECK; release_id is untouched. The marks are not kept:
-- re-applying the up migration derives 'admin' only from link_radio_play
-- audit rows and marks every other link 'matcher', so 'backfill' marks and
-- admin links without an audit row come back as 'matcher'.
SET LOCAL lock_timeout = '5s';

ALTER TABLE radio_plays DROP COLUMN release_link_source;
