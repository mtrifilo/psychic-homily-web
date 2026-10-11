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
--     thing to Postgres ARE and to Go RE2. Escapes of a letter or digit (\b, \d,
--     \m, \1, ...), POSIX classes ([[:alpha:]]) and (? groups are refused,
--     because the two engines read them differently or only one accepts them.
--     What remains (literals, escaped punctuation, ., anchors, groups,
--     alternation, quantifiers, bracket lists) matches identically in both.
--   * station_id NULL applies the row on every station; otherwise only to plays
--     that aired on that station.
--   * category 'station_element' and 'talk' exclude the whole play from every
--     ranking. 'placeholder' only disqualifies the matched field, so the play
--     still counts toward rankings that do not key on that field.
--
-- CHECK constraints rather than enum types, so value sets stay cheap to evolve.
-- Multi-statement file => golang-migrate runs it in a transaction.

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
    -- evaluating it against '' raises on invalid syntax, which rejects the row.
    CONSTRAINT radio_play_exclusions_pattern_form_check CHECK (
        CASE
            WHEN pattern_type = 'regex'
                THEN pattern !~ '\\[[:alnum:]]'
                     AND position('[[:' IN pattern) = 0
                     AND position('(?' IN pattern) = 0
                     AND ('' ~ pattern) IS NOT NULL
            ELSE pattern = radio_normalize_name(pattern)
        END
    ),
    CONSTRAINT radio_play_exclusions_rule_unique
        UNIQUE NULLS NOT DISTINCT (match_kind, pattern_type, pattern, station_id)
);

CREATE INDEX idx_radio_play_exclusions_station
    ON radio_play_exclusions(station_id) WHERE station_id IS NOT NULL;
