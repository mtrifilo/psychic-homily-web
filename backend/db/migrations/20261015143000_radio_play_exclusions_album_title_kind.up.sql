-- radio_play_exclusions.match_kind accepts 'album_title': the pattern is tested
-- against radio_normalize_name(radio_plays.album_title), under the matching
-- contract of the other kinds (catalog.RadioPlayExcludedSQL and
-- catalog.RadioPlayExclusions.Excludes, which must agree). A 'placeholder'
-- album row disqualifies only the album, so the play still counts toward the
-- artist, track and label rankings but not as release evidence; a
-- 'station_element' or 'talk' album row excludes the whole play, as for every
-- other kind.
--
-- Replacing the CHECK takes an ACCESS EXCLUSIVE lock on radio_play_exclusions
-- and re-checks its rows. lock_timeout bounds the wait for that lock, so a
-- long-running ranking read fails this migration instead of queueing every
-- later read of the table behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE radio_play_exclusions
    DROP CONSTRAINT radio_play_exclusions_match_kind_check,
    ADD CONSTRAINT radio_play_exclusions_match_kind_check
        CHECK (match_kind IN ('artist_name', 'track_title', 'label_name', 'album_title'));
