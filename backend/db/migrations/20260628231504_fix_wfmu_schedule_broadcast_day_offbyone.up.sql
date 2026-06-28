-- PSY-1283: Correct WFMU schedule slots stored one CALENDAR day early.
--
-- WFMU's wfmu.org/table is a BROADCAST-DAY grid (6am→6am): a slot in a column at a
-- post-midnight row (start < 06:00) airs the NEXT calendar day, not the day printed at
-- the top of its column. The PSY-1159 scraper stored the column day verbatim, so every
-- WFMU 91.1 flagship show with a pre-6am slot has its schedule's day_of_week off by one
-- (e.g. "Freeform Jazz Dance" stored Saturday but airs Sunday 3-6am). The matching code
-- fix (radio_wfmu_schedule.go: calendarWeekdayForSlot) ships in the same change, so every
-- future scrape stores the corrected day; this migration brings the ALREADY-stored rows
-- forward immediately, without waiting for (or depending on) the next live scrape. The impact
-- it addresses: WindowForDate(air_date) found no slot for these episodes' real weekday, leaving
-- them windowless (PSY-1238) — never "live". Correcting the schedule fixes the window for
-- FUTURE episodes at import and lets recently-aired ones heal (those re-listed within the
-- scheduleDerivedWindowMaxAgeDays=30 churn guard); historical episodes past that guard stay
-- windowless BY DESIGN (a frozen window is never recomputed against a since-churned schedule),
-- so this does not retroactively re-window old broadcasts. Also a root cause behind downstream
-- data-quality issues (PSY-1285/1286).
--
-- For each affected slot: day_of_week := (day_of_week + 1) mod 7. Applies the SAME rule the
-- fixed scraper now applies, so the migration and the next scrape can never disagree (a
-- scrape OVERWRITES the schedule, it does not re-shift, so there is no double-application).
--
-- Premise verified against the SOURCE OF TRUTH (the lesson from the reverted PSY-1253): a
-- read-only dry-run on stage compared each corrected day to the actual radio_episodes.air_date
-- weekday — all 14 affected shows matched (e.g. F4 Freeform Jazz Dance airs Sundays, CK Travel
-- Zone Tuesdays, T6 Thinking Hour Mondays), confirming WFMU's archive dates a post-midnight
-- broadcast to the next calendar day — i.e. the +1 shift aligns the schedule with how episodes
-- are actually dated. (The pre-6am rule mirrors radio_wfmu_schedule.go: calendarWeekdayForSlot.)
--
-- Scope — WFMU 91.1 flagship (slug 'wfmu'), schedule_locked = false. This DELIBERATELY skips
-- locked shows: an admin curated those (the API auto-locks on a structured-schedule edit, and
-- the PSY-1193 toggle can lock independently), and the weekly scrape honors the same contract,
-- so a one-time correction must not silently rewrite curated content. Locking does NOT by
-- itself guarantee a correct schedule — a show locked while still holding the scraper's
-- off-by-one would be skipped here AND by every future scrape — so this was checked on stage:
-- ZERO locked WFMU shows currently carry a pre-6am slot (all 14 affected are unlocked), so none
-- are missed. A locked-while-buggy show in any other environment needs manual re-curation.
--
-- NOT idempotent by nature: the start time is unchanged by the correction, so there is no
-- marker distinguishing corrected from uncorrected data and a re-run would shift again.
-- golang-migrate applies each version exactly once (tracked in schema_migrations); even an
-- accidental re-run self-heals on the next weekly/startup scrape, which overwrites with the
-- correct day. The EXISTS guard makes it a no-op for daytime-only shows and on environments
-- that were never affected (fresh CI DB), so the rows-affected count equals the number of
-- genuinely-corrected shows (14 on stage). The jsonb_typeof='number' guard leaves a non-numeric
-- day_of_week untouched, and the floor()/numeric cast tolerates a non-integer JSON number (e.g.
-- 6.0) instead of aborting — so one malformed slot can't fail the whole batch.
UPDATE radio_shows rs
   SET schedule = jsonb_set(
           rs.schedule,
           '{slots}',
           (
               SELECT jsonb_agg(
                          CASE
                              WHEN (arr.slot ->> 'start') < '06:00'
                                   AND jsonb_typeof(arr.slot -> 'day_of_week') = 'number'
                              THEN jsonb_set(
                                       arr.slot,
                                       '{day_of_week}',
                                       -- floor((...)::numeric)::int, not a bare ::int: a JSON
                                       -- number like 6.0 is typeof 'number' but '6.0'::int errors
                                       -- and would abort the whole migration (block the deploy).
                                       to_jsonb((floor((arr.slot ->> 'day_of_week')::numeric)::int + 1) % 7)
                                   )
                              ELSE arr.slot
                          END
                          ORDER BY arr.ord
                      )
               FROM jsonb_array_elements(rs.schedule -> 'slots')
                    WITH ORDINALITY AS arr(slot, ord)
           )
       )
  FROM radio_stations st
 WHERE rs.station_id = st.id
   AND st.slug = 'wfmu'
   AND rs.schedule_locked = false
   AND rs.schedule IS NOT NULL
   AND jsonb_typeof(rs.schedule -> 'slots') = 'array'
   AND EXISTS (
           SELECT 1
           FROM jsonb_array_elements(rs.schedule -> 'slots') AS e(slot)
           WHERE (e.slot ->> 'start') < '06:00'
             AND jsonb_typeof(e.slot -> 'day_of_week') = 'number'
       );
