-- Reverse 20261015143000_radio_play_exclusions_album_title_kind.up.sql. Every
-- album_title row is deleted first (seeded or added later), since the restored
-- CHECK refuses them.
SET LOCAL lock_timeout = '5s';

DELETE FROM radio_play_exclusions WHERE match_kind = 'album_title';

ALTER TABLE radio_play_exclusions
    DROP CONSTRAINT radio_play_exclusions_match_kind_check,
    ADD CONSTRAINT radio_play_exclusions_match_kind_check
        CHECK (match_kind IN ('artist_name', 'track_title', 'label_name'));
