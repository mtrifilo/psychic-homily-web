-- radio_track_title_marker_words is the alternation of marker words that
-- radio_track_strip_marker_groups matches as a group's first or last word.
CREATE OR REPLACE FUNCTION public.radio_track_title_marker_words()
  RETURNS text
  AS $$
    SELECT 'remix|remixes|rmx|mix|edit|version|rework|refix|reinterpretation|dub|dubmix|flip|bootleg|vip'
      || '|extended|alternate|alt|live|demo|instrumental|instr|acoustic|unplugged|acapella|session'
      || '|performance|rehearsal|outtake|excerpt|excerpts|exc|mono|stereo|remaster|remastered|lp|45'
      || '|unreleased|unissued|forthcoming|bonus|clean|explicit|dirty|vocal|feat|ft|featuring|prod'
  $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE;

-- radio_track_strip_marker_groups removes the run of trailing bracket groups,
-- (...), [...] or {...}, in which every group names a version, a format, a
-- release status or a credit; it stops at the first group from the end that
-- names none, so a subtitle such as "(Not Constantinople)" and everything
-- before it stays. Its input is already lower-cased and unaccented. A group is
-- a marker group when:
--   * its first word is a marker word: "(live at kexp)", "(feat. x)";
--   * its last word is a marker word, or "take", "cover" or "single", with an
--     optional number after it: "(ron trent remix)", "(alternate take)",
--     "(remix #2)";
--   * the whole group is a year, "radio", "original", "album" or
--     "a cappella": "(1969)", "(radio)";
--   * it starts with "listen:", a playlist page link label: "(listen: pop-up)".
-- Words are matched whole. Parts, reprises, intros and interludes are not
-- markers: "x (part 1)" and "x (part 2)" are different pieces.
-- A title that does not end in a closing bracket has no trailing group, so it
-- skips the pattern match.
CREATE OR REPLACE FUNCTION public.radio_track_strip_marker_groups(text)
  RETURNS text
  AS $$
    SELECT CASE WHEN $1 ~ '[\)\]\}]\s*$' THEN regexp_replace($1,
      '(?:\s*[\(\[\{](?:'
        || '[^\(\)\[\]\{\}a-z0-9]*(?:' || public.radio_track_title_marker_words() || ')\M[^\(\)\[\]\{\}]*'
        || '|[^\(\)\[\]\{\}]*\m(?:' || public.radio_track_title_marker_words()
        || '|take|cover|single)(?:\s*#?[0-9]+)?[^\(\)\[\]\{\}a-z0-9]*'
        || '|[^\(\)\[\]\{\}a-z0-9]*(?:(?:19|20)[0-9]{2}|radio|original|album|a cappella)[^\(\)\[\]\{\}a-z0-9]*'
        || '|listen:[^\(\)\[\]\{\}]*'
      || ')[\)\]\}])+\s*$', '') ELSE $1 END
  $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE STRICT;

-- radio_track_title_key is the title half of the track key. Stripping runs on
-- immutable_unaccent(lower(title)) before radio_normalize_name so the boundary
-- trim cannot eat a closing bracket first. Steps, in order:
--   1. an unbracketed credit to the end: " feat. x", " feat x", " ft. x",
--      " featuring x"
--   2. an unbracketed bare " ft x" to the end, unless digits or one of the
--      number words listed in the pattern come right before it, where "ft" is
--      the unit: "ten ft tall" and "50 ft queenie" are kept whole
--   3. trailing marker groups (radio_track_strip_marker_groups)
--   4. a dash tail carrying a keyword: " - live at x", " - radio edit"
--   5. step 3 again, for "x (demo) - live", whose bracket group is exposed
--      only once step 4 removes the dash tail
-- A title that strips to nothing keeps its unstripped normalized form.
-- Helpers are schema-qualified so the function also works where an index
-- expression is evaluated under a restricted search_path.
CREATE OR REPLACE FUNCTION public.radio_track_title_key(text)
  RETURNS text
  AS $$
    SELECT coalesce(
      nullif(public.radio_normalize_name(
        public.radio_track_strip_marker_groups(
          regexp_replace(
            public.radio_track_strip_marker_groups(
              regexp_replace(
                regexp_replace(
                  public.immutable_unaccent(lower($1)),
                  '\s+(featuring|feat\.?|ft\.)\s.*$', ''),
                '(?<=\S)(?<!\m(?:[0-9]+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety|hundred|thousand))\s+ft\s.*$', '')),
            '\s+[-\u2013\u2014]\s+.*\m(remix|edit|version|live|mono|stereo)\M.*$', ''))
      ), ''),
      public.radio_normalize_name($1)
    )
  $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE STRICT;
