-- Reverse 20261015210000_radio_plays_release_link_source.up.sql. Dropping the
-- column drops its CHECK; release_id is untouched. Re-applying the up
-- migration marks every link 'matcher' or, from the audit log, 'admin'.
SET LOCAL lock_timeout = '5s';

ALTER TABLE radio_plays DROP COLUMN release_link_source;
