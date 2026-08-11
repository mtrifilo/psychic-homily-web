'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { BracketLink } from '@/components/shared'
import { buildCitiesParam } from '@/components/filters/cityParams'
import { useSceneShows } from '../hooks'
import { showDisplayTitle, showHref } from '../sceneWeek'
import { formatDayCountLine, formatShowPrice, formatShowStartTime } from '../sceneDay'
import {
  SCENE_CALENDAR_ROW_CAP,
  SCENE_CALENDAR_WINDOW_DAYS,
  formatCalendarDateHeading,
  groupShowsByDate,
  resolveSceneTimeZone,
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
  const time = formatShowStartTime(show, sceneTimeZone)
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
function quietWindowCopy(city: string, roomCount: number, scope: 'tonight' | 'window'): string {
  const rooms = roomCount > 0 ? `the ${roomCount} ${city} rooms we track` : `the ${city} rooms we track`
  const when = scope === 'tonight' ? 'tonight' : 'in the next four weeks'
  return `Nothing on our calendar for ${rooms} ${when}. A room may have shows we have not listed.`
}

/**
 * Honest zero, plus exactly ONE step wider.
 *
 * The cheapest correct empty state in the prior art: state the zero, then offer
 * the next-widest view. No empty-state art, no "clear filters" dead end, and no
 * padding the window with other cities' shows.
 *
 * `withLinks` is false for the tonight bucket on a scene that DOES have later
 * rows: the window footer beneath the list already carries the same two
 * destinations, and a page offering "All upcoming in Phoenix" twice under one
 * heading gives a reader two identical targets to choose between.
 */
function QuietWindow({
  scene,
  scope,
  withLinks = true,
}: {
  scene: SceneDetail
  scope: 'tonight' | 'window'
  withLinks?: boolean
}) {
  const citiesParam = buildCitiesParam([{ city: scene.city, state: scene.state }])

  return (
    <div className="py-4">
      <p className="max-w-2xl text-sm text-muted-foreground">
        {quietWindowCopy(scene.city, scene.stats.venue_count, scope)}
      </p>
      {withLinks && (
        <div className="mt-3 flex flex-wrap items-center gap-4">
          <BracketLink
            label={`All upcoming in ${scene.city}`}
            href={`/shows?cities=${encodeURIComponent(citiesParam)}`}
          />
          <BracketLink label="Suggest a venue" href="/contribute" />
        </div>
      )}
    </div>
  )
}

/** One date's heading, count and rows. */
function SceneDateGroup({
  group,
  scene,
  isTonight,
  sceneTimeZone,
  quietLinks,
}: {
  group: SceneShowGroup
  scene: SceneDetail
  isTonight: boolean
  sceneTimeZone?: string
  quietLinks: boolean
}) {
  return (
    <section className="border-t border-border pt-4">
      <div className="flex items-baseline justify-between gap-4">
        <h3 className="font-mono text-[11px] uppercase tracking-widest">
          {formatCalendarDateHeading(group.date)}
          {isTonight && <span className="text-primary"> · TONIGHT</span>}
        </h3>
        <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
          {formatDayCountLine(group.shows.length)}
        </span>
      </div>
      {group.shows.length === 0 ? (
        <QuietWindow scene={scene} scope="tonight" withLinks={quietLinks} />
      ) : (
        <ul className="mt-1">
          {group.shows.map(show => (
            <SceneShowRow key={show.id} show={show} sceneTimeZone={sceneTimeZone} />
          ))}
        </ul>
      )}
    </section>
  )
}

/**
 * What the window is showing, and where the rest of it lives.
 *
 * NO `n of {upcoming_show_count}` here, deliberately. That field counts every
 * future show with no upper bound, so printing it as the denominator under a
 * heading that says "next 4 weeks" would tell the reader there are 308 more
 * shows in a window that does not contain them. It is the same defect PSY-1623
 * removed from two other surfaces, and the status band above refuses it for the
 * same reason. The truncated line states only what it can check: these are the
 * first rows, and the two links go where the rest is.
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
          ? `Showing the first ${rendered} in the next four weeks`
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
    limit: SCENE_CALENDAR_ROW_CAP,
  })

  const shows = useMemo(() => data?.shows ?? [], [data])

  return useMemo(() => {
    const timeZone = resolveSceneTimeZone(shows)
    // Read the clock only once data exists. The page server-renders with this
    // query unresolved, so the first client render matches the server's and no
    // date-derived value can differ across hydration.
    const tonight = shows.length > 0 ? sceneTonightDate(new Date(), timeZone) : null
    const dated = groupShowsByDate(shows, timeZone)

    // The endpoint stops at its row cap wherever that falls, which is usually
    // part-way through a date. That date's rows are a fragment, but its count
    // renders in the same register as every verified one, so it would state a
    // per-day total nobody checked. Drop it rather than qualify it; the footer
    // says the list was cut and the links go where the rest is. Never drop the
    // only group there is.
    const truncated = shows.length >= SCENE_CALENDAR_ROW_CAP
    const groups = truncated && dated.length > 1 ? dated.slice(0, -1) : dated
    const rendered = groups.reduce((n, group) => n + group.shows.length, 0)

    // The tonight bucket renders even when nothing is on it, so a reader
    // arriving before doors sees the answer to the question they came with
    // instead of the next date that happens to have a row. Only synthesized
    // when the zone is genuinely known. A guessed zone could tag the wrong
    // night, and a wrong TONIGHT is worse than none.
    //
    // Re-sorted rather than assumed to belong first: tonight is only earlier
    // than every dated group while the zone maths holds, and an ordering
    // invariant that depends on another computation being right is one that
    // eventually breaks silently.
    const withTonight =
      tonight && !groups.some(group => group.date === tonight)
        ? [{ date: tonight, shows: [] as SceneShowSummary[] }, ...groups].sort((a, b) =>
            a.date.localeCompare(b.date)
          )
        : groups

    return {
      isLoading,
      // A failed REFETCH keeps the rows it already has (TanStack v5 retains
      // `data` alongside `status: 'error'`), and a reconnect blip must not
      // replace four correct weeks with an apology. Only an error with nothing
      // to show is an error the reader needs to hear about.
      isError: isError && shows.length === 0,
      timeZone,
      tonight,
      // What the status band says about tonight, and NULL when the zone is
      // unknown so the band drops the clause rather than guessing a night.
      // This is the count the tonight GROUP renders, by construction: the same
      // number in both places, because the window opens at now and both are
      // answering "what is still ahead".
      tonightCount: tonight
        ? (withTonight.find(group => group.date === tonight)?.shows.length ?? 0)
        : null,
      groups: withTonight,
      rendered,
      truncated,
    }
  }, [shows, isLoading, isError])
}

export function SceneCalendar({ scene }: SceneCalendarProps) {
  const { isLoading, isError, timeZone, tonight, groups, rendered, truncated } =
    useSceneCalendarWindow(scene.slug)

  return (
    <div>
      <SceneWindowNav sceneSlug={scene.slug} />

      <section className="mt-6">
        {/* The mock draws a `[Full week in {city} →]` link on this header as
            well as in the footer. Only the footer one ships: the window strip
            three lines above already carries a THIS WEEK chip to the same page,
            and a second link with the footer's exact name AND href would give a
            reader using the links list two identical entries to choose between.
            The footer keeps its copy because a twenty-row list is long enough
            that the reader who reaches the bottom is a different reader. */}
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
            <QuietWindow scene={scene} scope="window" />
          ) : (
            groups.map(group => (
              <SceneDateGroup
                key={group.date}
                group={group}
                scene={scene}
                isTonight={group.date === tonight}
                sceneTimeZone={timeZone}
                quietLinks={groups.length === 1}
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
