'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { BracketLink } from '@/components/shared'
import { buildCitiesParam } from '@/components/filters/cityParams'
import { useSceneShows } from '../hooks'
import { showDisplayTitle, showHref } from '../sceneWeek'
import { formatDayCountLine, formatShowPrice, formatShowStartTime } from '../sceneDay'
import {
  SCENE_CALENDAR_FETCH_LIMIT,
  SCENE_CALENDAR_ROW_CAP,
  SCENE_CALENDAR_WINDOW_DAYS,
  formatCalendarDateHeading,
  groupShowsByDate,
  resolveSceneTimeZone,
  rowTimeZone,
  sceneTonightDate,
  venueSubLocality,
  type SceneShowGroup,
} from '../sceneCalendar'
import { ShowStatusBadge } from './sceneChrome'
import type { SceneDetail, SceneShowSummary } from '../types'

/**
 * The scene page's calendar: a window nav strip and four weeks of real rows.
 *
 * This module is the whole point of the redesign. The page used to print
 * `upcoming_show_count` and a link; the better page already existed at
 * `/scenes/{slug}/week` and nothing pointed at it. Rows come from the SAME
 * endpoint the Atlas preview reads, so a show cannot be named one thing here
 * and another there.
 */

interface SceneCalendarProps {
  scene: SceneDetail
}

/**
 * The window family, as a strip of path segments.
 *
 * Every window is a PATH SEGMENT, never a query param: four sites in the prior
 * art agree on that and it is what our own `/tonight` and `/week` already do.
 * The strip never degrades. A thin scene renders the full row, because a
 * window that is empty is still an answer.
 *
 * `this-weekend` has no route yet. It points at the week, which is the nearest
 * existing page that contains the weekend, rather than at a URL that 404s;
 * building the segment is Wave 2's job. `next-4-weeks` needs no segment at all:
 * it IS this page's default window, so the chip is the active state rather than
 * a link to somewhere else.
 *
 * Deliberately NOT drawn: the mock's `[Dated archive →]`. No archive index
 * route exists (only the dated `/scenes/{slug}/{YYYY-Wnn|YYYY-MM-DD}`
 * permalinks), and a chip pointing at a page this site 404s is a worse answer
 * than no chip.
 */
function SceneWindowNav({ sceneSlug }: { sceneSlug: string }) {
  const weekHref = `/scenes/${sceneSlug}/week`
  const windows = [
    { label: 'Tonight', href: `/scenes/${sceneSlug}/tonight` },
    { label: 'This weekend', href: weekHref },
    { label: 'This week', href: weekHref },
    { label: 'Next 4 weeks', href: null },
  ]

  return (
    <nav
      aria-label="Show windows"
      className="flex flex-wrap items-center gap-x-8 gap-y-2 border-y border-border py-3"
    >
      {windows.map(({ label, href }) =>
        href ? (
          <Link
            key={label}
            href={href}
            className="rounded-sm font-mono text-[11px] uppercase tracking-widest text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"
          >
            {label}
          </Link>
        ) : (
          // `aria-current="true"` and not `"page"`: the current PAGE is
          // /scenes/{slug}, and this chip marks a window within it.
          <span
            key={label}
            aria-current="true"
            className="font-mono text-[11px] uppercase tracking-widest text-primary underline decoration-primary decoration-2 underline-offset-[6px]"
          >
            {label}
          </span>
        )
      )}
    </nav>
  )
}

/**
 * One show: time, bill, price, room.
 *
 * The row grammar `/tonight` already proved, plus the sub-locality the prior
 * art puts on every row. Printing `(Mesa)` beside `(Phoenix)` is what lets a
 * metro scene read as a region for free, and it costs nothing because the
 * payload already carries `venue_city`.
 *
 * ONE outer link, to the show. The mock draws the room as its own link, but a
 * nested anchor is invalid HTML and a row with two targets is a row a reader
 * has to aim at; the room's own page is one hop away from the show.
 */
function SceneShowRow({
  show,
  sceneTimeZone,
}: {
  show: SceneShowSummary
  sceneTimeZone?: string
}) {
  // The zone this row's HEADING was bucketed in, or nothing. Printing a time
  // without it would fall through to `resolveShowTimezone`'s America/Phoenix
  // default and put an Arizona clock under a UTC-derived date. An absent time
  // is a smaller loss than a confident wrong one, and the column below is
  // width-reserved either way.
  const zone = rowTimeZone(show) ?? sceneTimeZone
  const time = zone ? formatShowStartTime(show, zone) : null
  const price = formatShowPrice(show)
  const subLocality = venueSubLocality(show)

  return (
    <li className="border-b border-border/40 last:border-b-0">
      <Link
        href={showHref(show)}
        className="group flex flex-col gap-0.5 py-2.5 transition-colors hover:bg-muted/40 sm:flex-row sm:items-baseline sm:gap-4"
      >
        {/* Reserved even when the instant is unusable, so the bills stay in one
            column rather than one row jumping left. */}
        <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground sm:w-20">
          {time ?? ''}
        </span>
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span
            className={`font-medium group-hover:underline ${
              show.is_cancelled ? 'text-muted-foreground line-through' : ''
            }`}
          >
            {showDisplayTitle(show)}
          </span>
          {show.is_cancelled && <ShowStatusBadge label="CANCELLED" />}
          {!show.is_cancelled && show.is_sold_out && <ShowStatusBadge label="SOLD OUT" />}
        </span>
        <span className="hidden flex-1 sm:block" aria-hidden="true" />
        {price && (
          <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground sm:w-20">
            {price}
          </span>
        )}
        {show.venue_name && (
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            {show.venue_name}
            {subLocality ? ` ${subLocality}` : ''}
          </span>
        )}
      </Link>
    </li>
  )
}

/**
 * The copy for a window with nothing on it.
 *
 * Never asserts that no show exists, only that none is on OUR calendar.
 * Coverage is a curated slice of each city's rooms, so "no shows" would be a
 * claim about the city this site is not entitled to make, and a local who knows
 * better would be right to stop trusting the rest of the page. The same rule
 * `/tonight`'s quiet night already follows.
 *
 * `roomCount` is named because it is what makes the sentence checkable: a
 * reader who knows the city can see exactly how wide the claim is.
 */
function quietWindowCopy(city: string, roomCount: number): string {
  const rooms = roomCount > 0 ? `the ${roomCount} ${city} rooms we track` : `the ${city} rooms we track`
  return `Nothing on our calendar for ${rooms} in the next four weeks. A room may have shows we have not listed.`
}

/**
 * Honest zero, plus exactly ONE step wider.
 *
 * The cheapest correct empty state in the prior art: state the zero, then offer
 * the next-widest view. No empty-state art, no "clear filters" dead end, and no
 * padding the window with other cities' shows.
 *
 * The week link rides here rather than only in the window footer, which does
 * not render when there are no rows to count. The scene with nothing on it is
 * the one that most needs an onward path, so it must not be the one state that
 * loses it.
 */
function QuietWindow({ scene }: { scene: SceneDetail }) {
  const citiesParam = buildCitiesParam([{ city: scene.city, state: scene.state }])

  return (
    <div className="py-4">
      <p className="max-w-2xl text-sm text-muted-foreground">
        {quietWindowCopy(scene.city, scene.stats.venue_count)}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-4">
        <BracketLink
          label={`Full week in ${scene.city}`}
          href={`/scenes/${scene.slug}/week`}
        />
        <BracketLink
          label={`All upcoming in ${scene.city}`}
          href={`/shows?cities=${encodeURIComponent(citiesParam)}`}
        />
        <BracketLink label="Suggest a venue" href="/contribute" />
      </div>
    </div>
  )
}

/**
 * One date's heading, count and rows.
 *
 * `countIsPartial` suppresses the count rather than qualifying it. It is set
 * when the endpoint's row cap landed inside this date, which happens on a very
 * dense night: the rows are real, but their number is not this date's total,
 * and printing it in the same register as every verified count would state a
 * per-day figure nobody checked.
 */
function SceneDateGroup({
  group,
  isTonight,
  sceneTimeZone,
  countIsPartial,
}: {
  group: SceneShowGroup
  isTonight: boolean
  sceneTimeZone?: string
  countIsPartial: boolean
}) {
  return (
    <section className="border-t border-border pt-4">
      <div className="flex items-baseline justify-between gap-4">
        <h3 className="font-mono text-[11px] uppercase tracking-widest">
          {formatCalendarDateHeading(group.date)}
          {isTonight && <span className="text-primary"> · TONIGHT</span>}
        </h3>
        {!countIsPartial && (
          <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
            {formatDayCountLine(group.shows.length)}
          </span>
        )}
      </div>
      <ul className="mt-1">
        {group.shows.map(show => (
          <SceneShowRow key={show.id} show={show} sceneTimeZone={sceneTimeZone} />
        ))}
      </ul>
    </section>
  )
}

/**
 * What the window is showing, and where the rest of it lives.
 *
 * The mock's wording, and the denominator is deliberate: `of {n} upcoming`
 * scopes the total to what is UPCOMING, not to what is in this window, so it
 * tells the reader the scale of what was cut without claiming the four weeks
 * contain all of it. That is a different thing from the PSY-1623 defect, which
 * was a count mislabelled as a WEEK.
 */
function WindowFooter({
  scene,
  rendered,
  truncated,
}: {
  scene: SceneDetail
  rendered: number
  truncated: boolean
}) {
  const citiesParam = buildCitiesParam([{ city: scene.city, state: scene.state }])

  return (
    <div className="mt-6 flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-t border-border pt-3">
      <p className="font-mono text-[11px] text-muted-foreground">
        {truncated
          ? `Showing ${rendered} of ${scene.stats.upcoming_show_count} upcoming`
          : 'Showing everything we have in the next four weeks'}
      </p>
      <div className="flex flex-wrap items-center gap-4">
        <BracketLink
          label={`Full week in ${scene.city}`}
          href={`/scenes/${scene.slug}/week`}
        />
        <BracketLink
          label={`All upcoming in ${scene.city}`}
          href={`/shows?cities=${encodeURIComponent(citiesParam)}`}
        />
      </div>
    </div>
  )
}

/**
 * The four-week window, resolved from the rows themselves.
 *
 * Exported so the page header can name the zone every time on the page is
 * printed in without opening a second request. TanStack Query serves both
 * callers from one cache entry.
 */
export function useSceneCalendarWindow(sceneSlug: string) {
  const { data, isLoading, isError } = useSceneShows(sceneSlug, {
    days: SCENE_CALENDAR_WINDOW_DAYS,
    limit: SCENE_CALENDAR_FETCH_LIMIT,
  })

  const fetched = useMemo(() => data?.shows ?? [], [data])

  return useMemo(() => {
    // One row over the cap means the endpoint had more to give. That is an
    // observed fact rather than the `length === cap` inference, which cannot
    // tell a cut list from a scene that happens to hold exactly that many and
    // so deletes a complete date from a page whose job is listing them.
    const truncated = fetched.length > SCENE_CALENDAR_ROW_CAP
    const shows = truncated ? fetched.slice(0, SCENE_CALENDAR_ROW_CAP) : fetched

    const timeZone = resolveSceneTimeZone(shows)
    // Read the clock only once data exists. The page server-renders with this
    // query unresolved, so the first client render matches the server's and no
    // date-derived value can differ across hydration.
    const tonight = shows.length > 0 ? sceneTonightDate(new Date(), timeZone) : null
    const groups = groupShowsByDate(shows, timeZone)

    // NO synthesized empty tonight bucket, deliberately, and this is the one
    // decision on this surface most worth not undoing.
    //
    // The window opens at `now` (`GetSceneUpcomingShows` filters
    // `event_date >= time.Now()`), so a show whose doors have opened is already
    // gone from the payload, and between midnight and 06:00 the night named by
    // the 6am boundary is YESTERDAY, which a forward window can never contain.
    // Drawing an empty bucket for that date published "Nothing on our calendar
    // for the 12 Phoenix rooms we track tonight" in our own voice, every
    // evening, on a night that demonstrably had shows, one click from
    // `/scenes/{slug}/tonight` which listed them. A bucket is only drawn for a
    // date the window actually answered for; the TONIGHT chip in the strip
    // above owns the question of what the whole night holds.
    const rendered = shows.length

    return {
      isLoading,
      // A failed REFETCH keeps the rows it already has (TanStack v5 retains
      // `data` alongside `status: 'error'`), and a reconnect blip must not
      // replace four correct weeks with an apology. Only an error with nothing
      // to show is an error the reader needs to hear about.
      isError: isError && shows.length === 0,
      timeZone,
      tonight,
      groups,
      rendered,
      truncated,
    }
  }, [fetched, isLoading, isError])
}

export function SceneCalendar({ scene }: SceneCalendarProps) {
  const { isLoading, isError, timeZone, tonight, groups, rendered, truncated } =
    useSceneCalendarWindow(scene.slug)

  return (
    <div>
      <SceneWindowNav sceneSlug={scene.slug} />

      <section className="mt-6">
        {/* The mock draws a `[Full week in {city} →]` link on this header as
            well as in the footer. Only ONE ships at a time: two links with the
            same name AND href in one render give a reader using the links list
            two identical entries to choose between. It rides at the foot of a
            populated list (where the reader who scrolled ends up) and inside
            the empty state (where there is no foot), so no state loses it. */}
        <h2 className="font-mono text-[11px] uppercase tracking-widest">
          Shows / next 4 weeks
          <span className="text-muted-foreground">
            {' · '}
            {scene.city} + metro
          </span>
        </h2>

        {/* Human voice, not machine voice. The prior art's own contrast: a page
            that says "we do our best to keep up" and one that says "this
            listing is automatically generated and not reviewed for accuracy"
            carry the same information and opposite signals about who is behind
            the page. */}
        <p className="mt-1 text-sm text-muted-foreground">
          Shows change all the time. We do our best to keep up, but check with the room
          before you head out.
        </p>

        <div className="mt-5 space-y-5">
          {isLoading ? (
            <p className="font-mono text-[11px] uppercase tracking-widest text-muted-foreground">
              Loading shows
            </p>
          ) : isError ? (
            /* A failed request is NOT an empty calendar. Falling through to the
               honest-zero copy would state, in our own voice and with a room
               count attached, that nothing is on tonight, a claim we did not
               check. Say what actually happened instead. */
            <p className="text-sm text-muted-foreground">
              We could not load this scene&apos;s calendar just now. Try again in a moment,
              or see{' '}
              <Link
                href={`/scenes/${scene.slug}/week`}
                className="underline underline-offset-4"
              >
                the full week in {scene.city}
              </Link>
              .
            </p>
          ) : groups.length === 0 ? (
            <QuietWindow scene={scene} />
          ) : (
            groups.map((group, i) => (
              <SceneDateGroup
                key={group.date}
                group={group}
                isTonight={group.date === tonight}
                sceneTimeZone={timeZone}
                // Only the LAST group can have been cut by the row cap, and
                // only when the cap was actually reached.
                countIsPartial={truncated && i === groups.length - 1}
              />
            ))
          )}
        </div>

        {!isLoading && groups.length > 0 && (
          <WindowFooter scene={scene} rendered={rendered} truncated={truncated} />
        )}
      </section>
    </div>
  )
}
