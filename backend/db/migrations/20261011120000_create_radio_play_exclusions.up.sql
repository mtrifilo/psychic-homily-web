-- radio_play_exclusions: rows that mark radio plays as noise (station elements,
-- talk) or mark one field of a play as a placeholder, so enrichment rankings
-- skip them. Rows are data: they are added and edited without a deploy, and the
-- raw radio_plays log is never modified by them.
--
-- Matching contract (implemented by catalog.RadioPlayExcludedSQL and
-- catalog.RadioPlayExclusions.Excludes, which must agree):
--   * the pattern is tested against radio_normalize_name(<field>) of the play;
--     an empty or NULL normalized value never matches any row.
--   * pattern_type 'exact' compares with =, so an exact pattern is stored in
--     normalized form (enforced below).
--   * pattern_type 'regex' compares with ~, and the pattern must mean the same
--     thing to Postgres ARE and to Go RE2. Refused here and by the Go loader:
--     an escape of anything but . ( ) [ ] * + ? | ^ $ \ / - }, a POSIX
--     bracket construct ([: [. [=), a (? group, a leading *, an opening brace
--     { (escaped or not), an ASCII capital (normalized values are lowercase, so it could
--     never match), a pattern that matches the empty string (it would match
--     every value), and more than 200 characters.
--   * station_id NULL applies the row on every station; otherwise only to plays
--     that aired on that station.
--   * category 'station_element' and 'talk' exclude the whole play from every
--     ranking. 'placeholder' only disqualifies the matched field, so the play
--     still counts toward rankings that do not key on that field.
--
-- CHECK constraints rather than enum types, so value sets stay cheap to evolve.

CREATE TABLE radio_play_exclusions (
    id BIGSERIAL PRIMARY KEY,
    match_kind VARCHAR(20) NOT NULL
        CONSTRAINT radio_play_exclusions_match_kind_check
        CHECK (match_kind IN ('artist_name', 'track_title', 'label_name')),
    pattern_type VARCHAR(10) NOT NULL
        CONSTRAINT radio_play_exclusions_pattern_type_check
        CHECK (pattern_type IN ('exact', 'regex')),
    pattern TEXT NOT NULL
        CONSTRAINT radio_play_exclusions_pattern_nonempty_check
        CHECK (pattern <> ''),
    category VARCHAR(20) NOT NULL
        CONSTRAINT radio_play_exclusions_category_check
        CHECK (category IN ('station_element', 'talk', 'placeholder')),
    station_id BIGINT REFERENCES radio_stations(id) ON DELETE CASCADE,
    reason TEXT NOT NULL
        CONSTRAINT radio_play_exclusions_reason_nonempty_check
        CHECK (btrim(reason) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- CASE fixes the evaluation order: an exact pattern such as 'no label (boot'
    -- is not a valid regex and must never reach the ~ operator. For a regex,
    -- evaluating it against '' raises on invalid syntax, which rejects the row,
    -- and a match on '' rejects a pattern that would match every value.
    CONSTRAINT radio_play_exclusions_pattern_form_check CHECK (
        CASE
            WHEN pattern_type = 'regex'
                THEN char_length(pattern) <= 200
                     AND pattern !~ '\\[^\]\[.()*+?|^$\\{}/-]'
                     AND position('[:' IN pattern) = 0
                     AND position('[.' IN pattern) = 0
                     AND position('[=' IN pattern) = 0
                     AND position('(?' IN pattern) = 0
                     AND left(pattern, 1) <> '*'
                     AND position('{' IN pattern) = 0
                     AND pattern !~ '[ABCDEFGHIJKLMNOPQRSTUVWXYZ]'
                     AND ('' ~ pattern) = false
            ELSE pattern = radio_normalize_name(pattern)
        END
    ),
    CONSTRAINT radio_play_exclusions_rule_unique
        UNIQUE NULLS NOT DISTINCT (match_kind, pattern_type, pattern, station_id)
);

-- The ranking predicate probes station-scoped rows once per play by
-- station_id; without this index each probe scans the whole table.
CREATE INDEX idx_radio_play_exclusions_station
    ON radio_play_exclusions(station_id) WHERE station_id IS NOT NULL;
