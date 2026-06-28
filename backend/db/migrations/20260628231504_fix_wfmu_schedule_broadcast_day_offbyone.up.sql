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
-- Scope — WFMU 91.1 flagship (slug 'wfmu'), schedule_locked = false. Locked shows are
-- admin-curated (the API auto-locks on a manual schedule edit), so they are guaranteed NOT
-- to carry the scraper's off-by-one and must not be silently rewritten — the same contract
-- the weekly scrape honors. Restricting to unlocked rows also makes this safe: at apply time
-- every unlocked WFMU schedule is scraper-produced (buggy) data, never a hand-corrected value.
--
-- NOT idempotent by nature: the start time is unchanged by the correction, so there is no
-- marker distinguishing corrected from uncorrected data and a re-run would shift again.
-- golang-migrate applies each version exactly once (tracked in schema_migrations); even an
-- accidental re-run self-heals on the next weekly/startup scrape, which overwrites with the
-- correct day. The EXISTS guard makes it a no-op for daytime-only shows and on environments
-- that were never affected (fresh CI DB), so the rows-affected count equals the number of
-- genuinely-corrected shows (≈14 on stage). The jsonb_typeof guard on day_of_week leaves any
-- malformed slot untouched rather than aborting the whole batch.
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
                                       to_jsonb(((arr.slot ->> 'day_of_week')::int + 1) % 7)
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
