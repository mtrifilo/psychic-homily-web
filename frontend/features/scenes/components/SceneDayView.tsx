import Link from 'next/link'
// Imported directly, not through the `components/shared` barrel: this page had
// no dependency on that barrel at all, and going through it would pull all ~30
// shared components into this route's module graph to use one of them.
import { ShareButton } from '@/components/shared/ShareButton'
import { ShowPrice } from '@/components/shared/ShowPrice'
import { showDisplayTitle, showHref } from '../sceneWeek'
import {
  dayShows,
  dayTrackedVenues,
  formatDayChip,
  formatDayCountLine,
  formatDayFull,
  formatPointerDay,
  formatShowStartTime,
  orderNightShows,
  type SceneDayResponse,
  type SceneDayShow,
} from '../sceneDay'
import {
  SCENE_NAV_CHIP_CLASS,
  RoomList,
  SceneBreadcrumb,
  SceneCityHeading,
  ShowStatusBadge,
  TrackedRoomsFooter,
} from './sceneChrome'

/**
 * One show, time first.
 *
 * The time leads because a single night is read as a schedule — the reader has
 * already chosen the date and is choosing an hour. The week view, which is read
 * as seven buckets, has no such column and should not grow one.
 *
 * Layout differs by breakpoint on purpose, matching the week row: at mobile
 * widths a right-hand venue column cannot survive real band names, so the
 * metadata stacks underneath instead.
 */
function ShowRow({ show, sceneTimezone }: { show: SceneDayShow; sceneTimezone?: string }) {
  const time = formatShowStartTime(show, sceneTimezone)

  return (
    <li className="border-b border-border/40">
      <Link
        href={showHref(show)}
        className="group flex flex-col gap-0.5 py-3 transition-colors hover:bg-muted/40 sm:flex-row sm:items-baseline sm:gap-4"
      >
        {/* Reserved even when the instant is unusable, so the bills stay in one
            column rather than one row jumping left. */}
        <span className="shrink-0 font-mono text-xs text-muted-foreground sm:w-24">
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
        <ShowPrice
          show={show}
          className="shrink-0 font-mono text-xs text-muted-foreground"
        />
        {show.venue_name && (
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            {show.venue_name}
          </span>
        )}
      </Link>
    </li>
  )
}

/**
 * The copy for a night with nothing on it.
 *
 * Never asserts that no show exists — only that none is on OUR calendar.
 * Coverage is a curated slice of each city's rooms, so "no shows tonight" would
 * be a claim about the city this site is not entitled to make, and a local who
 * knows better would be right to stop trusting the rest of the page.
 */
function quietNightCopy(day: SceneDayResponse, hasUpcoming: boolean): string {
  const when = day.is_tonight ? 'tonight' : `on ${formatDayFull(day.date)}`
  const base = `Nothing on our calendar for the ${day.city} rooms we track ${when}`

  // "or in the next few weeks" is a claim about NOW, and only a night that has
  // not already happened has the standing to make it — the look-ahead behind it
  // starts at the day being viewed, so on an archived page that window closed
  // years ago and was never re-checked. A past night falls back to the plain
  // sentence rather than growing a variant of its own: the copy here is locked,
  // and substituting the date is the one liberty it grants.
  if (day.is_past_day || hasUpcoming) {
    return `${base}. A room may have a show we haven't listed.`
  }
  return `${base}, or in the next few weeks. A room may have shows we haven't listed.`
}

function EmptyNight({ day, weekHref }: { day: SceneDayResponse; weekHref: string }) {
  const nextShow = day.next_show
  const rooms = dayTrackedVenues(day)

  return (
    <div className="py-8">
      <p className="max-w-2xl text-muted-foreground">
        {quietNightCopy(day, Boolean(nextShow))}
      </p>

      {nextShow && (
        <p className="mt-5">
          <Link
            href={showHref(nextShow)}
            className="text-primary underline underline-offset-4"
          >
            Next on our calendar:{' '}
            {formatPointerDay(day.date, nextShow.event_date, day.is_tonight)},{' '}
            {showDisplayTitle(nextShow)}
            {nextShow.venue_name ? ` at ${nextShow.venue_name}` : ''} →
          </Link>
        </p>
      )}

      <p className="mt-2">
        <Link href={weekHref} className="underline underline-offset-4">
          Full week in {day.city} →
        </Link>
      </p>

      {rooms.length > 0 && (
        <section className="mt-8">
          {/* The reader is being told we have nothing; the least we owe them is
              a path into each room's page here (PSY-1733). */}
          <h2 className="font-mono text-[11px] tracking-widest text-muted-foreground">
            CHECK THE ROOMS DIRECTLY
          </h2>
          <RoomList venues={rooms.map(({ name, slug }) => ({ name, slug }))} />
        </section>
      )}

      {/* The dead-quiet ask, and ONLY that: a scene with nothing ahead of it is
          the one most likely to be missing a room. An archived empty Tuesday
          says nothing about the scene's current calendar, so it must not
          solicit on that basis — `!nextShow` alone would, since the server
          never sends a pointer for a past date.

          A sibling of the rooms block, not a child of it: a scene whose room
          list came back empty is likelier still to be missing one, which is
          exactly when nesting this would have hidden it. */}
      {!nextShow && !day.is_past_day && (
        <Link
          href="/contribute"
          className="mt-3 inline-block text-sm text-muted-foreground hover:underline"
        >
          Missing a room? Suggest a venue →
        </Link>
      )}
    </div>
  )
}

export function SceneDayView({ day }: { day: SceneDayResponse }) {
  // On the LIVE night the rows a reader can still get to lead, and the ones
  // already under way sink beneath them in the order they started (user
  // decision, PSY-1969). Nothing is DROPPED, so the count below is still a
  // count of everything listed; an archive or future night is untouched, since
  // a schedule is read in clock order.
  const shows = orderNightShows(dayShows(day), day.is_tonight)
  const rooms = dayTrackedVenues(day)
  const total = shows.length

  // The rolling week for tonight, the dated week permalink otherwise — a page
  // about a night two months ago must not link to whatever week it happens to
  // be now.
  const weekHref = day.is_tonight
    ? `/scenes/${day.slug}/week`
    : `/scenes/${day.slug}/${day.iso_week}`

  return (
    <div className="mx-auto w-full max-w-4xl px-4 pb-16 pt-8 md:px-6">
      <SceneBreadcrumb slug={day.slug} sceneName={day.scene_name} />

      <header className="mt-2">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <SceneCityHeading city={day.city} state={day.state} />

          {/* The adjacent-day chips render only when the server offered a
              date. At the edges of the servable window there is no next day to
              go to, and a chip pointing at a URL this site 404s is a worse
              answer than no chip. */}
          <div className="flex gap-2">
            {day.prev_date && (
              <Link
                href={`/scenes/${day.slug}/${day.prev_date}`}
                className={SCENE_NAV_CHIP_CLASS}
                rel="prev"
              >
                ← {formatDayChip(day.prev_date)}
              </Link>
            )}
            <Link href={weekHref} className={SCENE_NAV_CHIP_CLASS}>
              Full week
            </Link>
            {day.next_date && (
              <Link
                href={`/scenes/${day.slug}/${day.next_date}`}
                className={SCENE_NAV_CHIP_CLASS}
                rel="next"
              >
                {formatDayChip(day.next_date)} →
              </Link>
            )}
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
          <p className="font-mono text-sm">
            {day.is_tonight && 'Tonight — '}
            {formatDayFull(day.date)}
            {'   ·   '}
            {formatDayCountLine(total)}
          </p>
          {/* Always the DATED permalink, even when this renders at the rolling
              `/scenes/{slug}/tonight` URL. Sharing the rolling URL would hand a
              friend a page whose contents change out from under the message
              tomorrow — the same divergence the weekly page's control makes,
              between the canonical and what the address bar shows. */}
          <ShareButton
            path={`/scenes/${day.slug}/${day.date}`}
            ariaLabel="Share this night"
          />
        </div>

        {/* Load-bearing, not filler: coverage is a curated slice (11 rooms in
            Chicago, not all of Chicago). A page that implied full city coverage
            would be false, and a local would notice immediately. The quiet-night
            body below says the same thing in its own words, so it is not
            repeated there. */}
        {total > 0 && (
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
            Every show {day.is_tonight ? 'tonight' : 'that day'} at the {day.city} rooms we
            track. Not a complete city listing — see the rooms below.
          </p>
        )}
      </header>

      <div className="mt-6 border-t-2 border-foreground" />

      {total === 0 ? (
        <EmptyNight day={day} weekHref={weekHref} />
      ) : (
        <ul className="pt-2">
          {shows.map(show => (
            <ShowRow key={show.id} show={show} sceneTimezone={day.timezone} />
          ))}
        </ul>
      )}

      {/* Only under a listing. EmptyNight renders the same rooms under its own
          heading (CHECK THE ROOMS DIRECTLY) — both paths share RoomList so the
          /venues/{slug} destination cannot drift (PSY-1733). Project name+slug
          only: SceneTrackedVenue also carries website, which is intentionally
          not a footer href. */}
      {total > 0 && (
        <TrackedRoomsFooter
          city={day.city}
          rooms={rooms.map(({ name, slug }) => ({ name, slug }))}
        />
      )}
    </div>
  )
}
