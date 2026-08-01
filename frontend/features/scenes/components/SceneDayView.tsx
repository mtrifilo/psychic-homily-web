import Link from 'next/link'
import { showDisplayTitle, showHref } from '../sceneWeek'
import {
  countDayShows,
  dayShows,
  dayTrackedVenues,
  formatDayChip,
  formatDayCountLine,
  formatDayFull,
  formatPointerDay,
  formatShowPrice,
  formatShowStartTime,
  venueWebsiteHref,
  type SceneDayResponse,
  type SceneDayShow,
  type SceneTrackedVenue,
} from '../sceneDay'

function StatusBadge({ label, tone }: { label: string; tone: 'destructive' | 'muted' }) {
  const toneClass =
    tone === 'destructive'
      ? 'border-destructive text-destructive'
      : 'border-muted-foreground text-muted-foreground'
  return (
    <span
      className={`shrink-0 rounded-sm border px-1.5 py-px font-mono text-[10px] leading-4 tracking-wide ${toneClass}`}
    >
      {label}
    </span>
  )
}

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
  const price = formatShowPrice(show)

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
          {show.is_cancelled && <StatusBadge label="CANCELLED" tone="destructive" />}
          {!show.is_cancelled && show.is_sold_out && (
            <StatusBadge label="SOLD OUT" tone="destructive" />
          )}
        </span>
        <span className="hidden flex-1 sm:block" aria-hidden="true" />
        {price && (
          <span className="shrink-0 font-mono text-xs text-muted-foreground">{price}</span>
        )}
        {show.venue_name && (
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            {show.venue_name}
          </span>
        )}
      </Link>
    </li>
  )
}

/** One tracked room: its own site when we have one, its page here otherwise. */
function RoomLink({ venue }: { venue: SceneTrackedVenue }) {
  const website = venueWebsiteHref(venue.website)
  if (website) {
    return (
      <a
        href={website}
        target="_blank"
        rel="noopener noreferrer nofollow"
        className="underline underline-offset-4 hover:text-primary"
      >
        {venue.name} ↗
      </a>
    )
  }
  if (venue.slug) {
    return (
      <Link
        href={`/venues/${venue.slug}`}
        className="underline underline-offset-4 hover:text-primary"
      >
        {venue.name}
      </Link>
    )
  }
  // No site and no page of its own: name it anyway. The list's job is to tell
  // the reader WHICH rooms this page speaks for, and dropping one would
  // misstate the coverage it is there to disclose.
  return <span>{venue.name}</span>
}

/** `A · B · C`, with each room linked. */
function RoomList({ venues }: { venues: SceneTrackedVenue[] }) {
  return (
    <p className="mt-2 text-sm leading-relaxed">
      {venues.map((venue, i) => (
        <span key={venue.slug || venue.name}>
          {i > 0 && <span className="text-muted-foreground"> · </span>}
          <RoomLink venue={venue} />
        </span>
      ))}
    </p>
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
  if (hasUpcoming) {
    return `Nothing on our calendar for the ${day.city} rooms we track ${when}. A room may have a show we haven't listed.`
  }
  return `Nothing on our calendar for the ${day.city} rooms we track ${when}, or in the next few weeks. A room may have shows we haven't listed.`
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
            Next on our calendar: {formatPointerDay(day.date, nextShow.event_date)},{' '}
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
              the means to check for themselves. */}
          <h2 className="font-mono text-[11px] tracking-widest text-muted-foreground">
            CHECK THE ROOMS DIRECTLY
          </h2>
          <RoomList venues={rooms} />
          {!nextShow && (
            <Link
              href="/contribute"
              className="mt-3 inline-block text-sm text-muted-foreground hover:underline"
            >
              Missing a room? Suggest a venue →
            </Link>
          )}
        </section>
      )}
    </div>
  )
}

export function SceneDayView({ day }: { day: SceneDayResponse }) {
  const shows = dayShows(day)
  const rooms = dayTrackedVenues(day)
  const total = countDayShows(day)

  // The rolling week for tonight, the dated week permalink otherwise — a page
  // about a night two months ago must not link to whatever week it happens to
  // be now.
  const weekHref = day.is_tonight
    ? `/scenes/${day.slug}/week`
    : `/scenes/${day.slug}/${day.iso_week}`

  return (
    <div className="mx-auto w-full max-w-4xl px-4 pb-16 pt-8 md:px-6">
      <nav aria-label="Breadcrumb" className="font-mono text-[11px] text-muted-foreground">
        <Link href="/scenes" className="hover:underline">
          Scenes
        </Link>
        {'  /  '}
        <Link href={`/scenes/${day.slug}`} className="hover:underline">
          {day.scene_name}
        </Link>
      </nav>

      <header className="mt-2">
        <div className="flex flex-wrap items-end justify-between gap-4">
          {/* City at display scale, state in mono alongside — the same header as
              the week view, because this page is its sibling and arrives cold
              from a shared link just as often. "Columbus" alone is ambiguous. */}
          <h1 className="flex items-baseline gap-3 text-4xl font-bold tracking-tight md:text-5xl">
            {day.city}
            <span className="font-mono text-base font-normal tracking-wide text-muted-foreground">
              {day.state}
            </span>
          </h1>

          <div className="flex gap-2">
            <Link
              href={`/scenes/${day.slug}/${day.prev_date}`}
              className="rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50"
              rel="prev"
            >
              ← {formatDayChip(day.prev_date)}
            </Link>
            <Link
              href={weekHref}
              className="rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50"
            >
              Full week
            </Link>
            <Link
              href={`/scenes/${day.slug}/${day.next_date}`}
              className="rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50"
              rel="next"
            >
              {formatDayChip(day.next_date)} →
            </Link>
          </div>
        </div>

        <p className="mt-3 font-mono text-sm">
          {day.is_tonight && 'Tonight — '}
          {formatDayFull(day.date)}
          {'   ·   '}
          {formatDayCountLine(total)}
        </p>

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

      {total === 0 || shows.length === 0 ? (
        <EmptyNight day={day} weekHref={weekHref} />
      ) : (
        <ul className="pt-2">
          {shows.map(show => (
            <ShowRow key={show.id} show={show} sceneTimezone={day.timezone} />
          ))}
        </ul>
      )}

      {shows.length > 0 && rooms.length > 0 && (
        <footer className="mt-12">
          <div className="border-t-2 border-foreground" />
          <h2 className="pt-4 font-mono text-[11px] tracking-widest text-muted-foreground">
            ROOMS WE TRACK IN {day.city.toUpperCase()}
          </h2>
          <p className="mt-2 text-sm leading-relaxed">
            {rooms.map(v => v.name).join(' · ')}
          </p>
          <Link
            href="/contribute"
            className="mt-2 inline-block text-sm text-muted-foreground hover:underline"
          >
            Missing a room? Suggest a venue →
          </Link>
        </footer>
      )}
    </div>
  )
}
