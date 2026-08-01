import Link from 'next/link'
// Imported directly, not through the `components/shared` barrel: this page had
// no dependency on that barrel at all, and going through it would pull all ~30
// shared components into this route's module graph to use one of them.
import { ShareButton } from '@/components/shared/ShareButton'
import {
  countShows,
  formatDayHeading,
  formatWeekRange,
  showDisplayTitle,
  showHref,
  type SceneWeekResponse,
  type SceneWeekShow,
} from '../sceneWeek'

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
 * One show. Bill leads; venue is metadata.
 *
 * Layout differs by breakpoint on purpose: on desktop the venue is a
 * right-aligned column, but at mobile widths (~358px of content) a right column
 * cannot survive real band names — `Slightly Stoopid, The Elovaters, Bumpin
 * Uglies` consumes the row on its own — so the venue stacks underneath instead.
 */
function ShowRow({ show }: { show: SceneWeekShow }) {
  return (
    <li className="border-b border-border/40 last:border-b-0">
      <Link
        href={showHref(show)}
        className="group flex flex-col gap-0.5 py-2.5 transition-colors hover:bg-muted/40 sm:flex-row sm:items-center sm:gap-4"
      >
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
        {show.venue_name && (
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            {show.venue_name}
          </span>
        )}
      </Link>
    </li>
  )
}

function DayGroup({ date, shows }: { date: string; shows: SceneWeekShow[] }) {
  return (
    <section className="pt-6">
      <div className="flex items-center justify-between pb-2">
        <h2 className="font-mono text-xs tracking-widest text-muted-foreground">
          {formatDayHeading(date)}
        </h2>
        <span className="font-mono text-xs text-muted-foreground">{shows.length}</span>
      </div>
      <div className="border-t border-border" />
      {/* A quiet day is conveyed by the `0` in the heading above; repeating it
          as a sentence adds nothing. In a sparse scene that redundancy is most
          of the page — six of seven days on a real Phoenix week — so the empty
          day is left as just its rule and count. The day still RENDERS, because
          showing the whole week is what tells a reader we checked. */}
      {shows.length > 0 && (
        <ul>
          {shows.map(show => (
            <ShowRow key={show.id} show={show} />
          ))}
        </ul>
      )}
    </section>
  )
}

export function SceneWeekView({ week }: { week: SceneWeekResponse }) {
  const days = week.days ?? []
  const rooms = week.tracked_venues ?? []
  const total = countShows(week)

  return (
    <div className="mx-auto w-full max-w-4xl px-4 pb-16 pt-8 md:px-6">
      <nav aria-label="Breadcrumb" className="font-mono text-[11px] text-muted-foreground">
        <Link href="/scenes" className="hover:underline">
          Scenes
        </Link>
        {'  /  '}
        <Link href={`/scenes/${week.slug}`} className="hover:underline">
          {week.scene_name}
        </Link>
      </nav>

      <header className="mt-2">
        <div className="flex flex-wrap items-end justify-between gap-4">
          {/* City at display scale, state in mono alongside. The page is built
              for cold arrivals from a shared link, where "Columbus" or
              "Portland" are genuinely ambiguous — so the state has to be on the
              page, not only in the breadcrumb. Setting it at display size would
              blunt the one element that must survive a skim. */}
          <h1 className="flex items-baseline gap-3 text-4xl font-bold tracking-tight md:text-5xl">
            {week.city}
            <span className="font-mono text-base font-normal tracking-wide text-muted-foreground">
              {week.state}
            </span>
          </h1>

          <div className="flex gap-2">
            <Link
              href={`/scenes/${week.slug}/${week.prev_week}`}
              className="flex-1 rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50 sm:flex-none"
              rel="prev"
            >
              ← {week.prev_week}
            </Link>
            {/* The reciprocal of the nightly page's "Full week" chip. Kept on
                archived weeks too: a reader who lands on last March still wants
                the way back to what is on now. */}
            <Link
              href={`/scenes/${week.slug}/tonight`}
              className="flex-1 rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50 sm:flex-none"
            >
              Tonight
            </Link>
            <Link
              href={`/scenes/${week.slug}/${week.next_week}`}
              className="flex-1 rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50 sm:flex-none"
              rel="next"
            >
              {week.next_week} →
            </Link>
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
          <p className="font-mono text-sm">
            {formatWeekRange(week.start_date, week.end_date)}
            {'   ·   '}
            {total} {total === 1 ? 'show' : 'shows'}
            {rooms.length > 0 && `   ·   ${rooms.length} rooms tracked`}
          </p>
          {/* Always the DATED permalink, even when this renders at the rolling
              `/scenes/{slug}/week` URL. Sharing the rolling URL would hand a
              friend a page whose contents change out from under the message
              next Monday — the one divergence on this page between the
              canonical and what the address bar shows. */}
          <ShareButton
            path={`/scenes/${week.slug}/${week.iso_week}`}
            ariaLabel="Share this week"
          />
        </div>

        {/* Load-bearing, not filler: coverage is a curated slice (11 rooms in
            Chicago, not all of Chicago). A page that implied full city coverage
            would be false, and a local would notice immediately. */}
        <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
          Every show at the {week.city} rooms we track. Not a complete city listing — see the
          rooms below.
        </p>
      </header>

      <div className="mt-6 border-t-2 border-foreground" />

      {total === 0 ? (
        <p className="py-10 text-muted-foreground">
          No shows at the {week.city} rooms we track this week.{' '}
          <Link href={`/scenes/${week.slug}/${week.next_week}`} className="underline">
            Try next week
          </Link>
          .
        </p>
      ) : (
        days.map(day => <DayGroup key={day.date} date={day.date} shows={day.shows ?? []} />)
      )}

      {rooms.length > 0 && (
        <footer className="mt-12">
          <div className="border-t-2 border-foreground" />
          <h2 className="pt-4 font-mono text-[11px] tracking-widest text-muted-foreground">
            ROOMS WE TRACK IN {week.city.toUpperCase()}
          </h2>
          <p className="mt-2 text-sm leading-relaxed">{rooms.join(' · ')}</p>
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
