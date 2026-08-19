import Link from 'next/link'
// Imported directly, not through the `components/shared` barrel: this page has
// no other dependency on that barrel, and going through it would pull all ~30
// shared components into this route's module graph to use one of them.
import { ShareButton } from '@/components/shared/ShareButton'
import { formatDayHeading, type SceneWeekDay } from '../sceneWeek'
import {
  SCENE_WINDOW_LABEL,
  SCENE_WINDOW_ORDER,
  allUpcomingHref,
  formatWindowRange,
  sceneWindowHref,
  type SceneWindowData,
  type SceneWindowKey,
} from '../sceneWindow'
import { SceneWeekShowRow } from './SceneWeekView'
import {
  SCENE_NAV_CHIP_CLASS,
  SceneBreadcrumb,
  SceneCityHeading,
  TrackedRoomsFooter,
} from './sceneChrome'

/**
 * A window of a scene's calendar that spans more than one night: the shared
 * body behind `/this-weekend` and `/next-4-weeks`.
 *
 * Sibling of `SceneWeekView` by construction, not by resemblance — same
 * breadcrumb, same city heading, same chip class, same row, same rooms footer.
 * The two differ only in which stretch of time they bound, so anything a reader
 * could notice moving between them is a bug.
 */

/**
 * The other windows in the family, as chips.
 *
 * The CURRENT window is not drawn. That follows the sibling pages, whose strips
 * are a set of places to go rather than a tab bar — `/week` offers "Tonight",
 * `/tonight` offers "Full week", and neither restates where the reader already
 * is. Repointing the scene page's own strip is PSY-1850; unifying all four
 * strips into one component is PSY-1786.
 */
function SceneWindowNav({ slug, current }: { slug: string; current: SceneWindowKey }) {
  return (
    <nav aria-label="Show windows" className="flex gap-2">
      {SCENE_WINDOW_ORDER.filter(key => key !== current).map(key => (
        <Link
          key={key}
          href={sceneWindowHref(slug, key)}
          className={`flex-1 sm:flex-none ${SCENE_NAV_CHIP_CLASS}`}
        >
          {SCENE_WINDOW_LABEL[key]}
        </Link>
      ))}
    </nav>
  )
}

/**
 * One date's heading, count and rows.
 *
 * `countIsPartial` suppresses the count rather than qualifying it. It is set
 * when the row cap landed inside this date: the rows are real, but their number
 * is not this date's total, and printing it in the same register as every
 * verified count would state a per-day figure nobody checked.
 */
function DayGroup({
  day,
  countIsPartial,
}: {
  day: SceneWeekDay
  countIsPartial: boolean
}) {
  const shows = day.shows ?? []

  return (
    <section className="pt-6">
      <div className="flex items-center justify-between pb-2">
        <h2 className="font-mono text-xs tracking-widest text-muted-foreground">
          {formatDayHeading(day.date)}
        </h2>
        {!countIsPartial && (
          <span className="font-mono text-xs text-muted-foreground">{shows.length}</span>
        )}
      </div>
      <div className="border-t border-border" />
      {shows.length > 0 && (
        <ul>
          {shows.map(show => (
            <SceneWeekShowRow key={show.id} show={show} />
          ))}
        </ul>
      )}
    </section>
  )
}

/**
 * A window with nothing in it.
 *
 * Never asserts that no show exists — only that none is on OUR calendar.
 * Coverage is a curated slice of each city's rooms, so "no shows this weekend"
 * would be a claim about the city this site is not entitled to make, and a local
 * who knows better would be right to stop trusting the rest of the page.
 *
 * An empty window is still an answer, so it gets the same chrome as a full one
 * plus exactly one way onward: the next window out. Offering the whole family
 * again here would just restate the strip above it.
 */
function QuietWindow({ data }: { data: SceneWindowData }) {
  const label = SCENE_WINDOW_LABEL[data.window].toLowerCase()

  return (
    <div className="py-8">
      <p className="max-w-2xl text-muted-foreground">
        Nothing on our calendar for the {data.city} rooms we track {label}. A room may
        have a show we haven&apos;t listed.
      </p>

      {/* Exactly one step onward. The widest window has nothing wider to offer,
          so it falls back to the city's whole upcoming listing rather than
          leaving a dead end — the state most likely to be reached by a scene we
          are simply missing rooms for. */}
      <p className="mt-5">
        {data.widerWindow ? (
          <Link
            href={sceneWindowHref(data.slug, data.widerWindow)}
            className="text-primary underline underline-offset-4"
          >
            Try {SCENE_WINDOW_LABEL[data.widerWindow].toLowerCase()} in {data.city} →
          </Link>
        ) : (
          <Link
            href={allUpcomingHref(data.city, data.state)}
            className="text-primary underline underline-offset-4"
          >
            All upcoming in {data.city} →
          </Link>
        )}
      </p>

      <p className="mt-3">
        <Link
          href="/contribute"
          className="text-sm text-muted-foreground hover:underline"
        >
          Missing a room? Suggest a venue →
        </Link>
      </p>
    </div>
  )
}

export function SceneWindowView({ data }: { data: SceneWindowData }) {
  const { days, rendered, truncated } = data
  const range = formatWindowRange(days)
  // A window with no DAYS at all and one whose days are all empty are the same
  // answer to the reader, and both must reach the quiet copy rather than
  // rendering a run of bare rules.
  const isQuiet = rendered === 0

  return (
    <div className="mx-auto w-full max-w-4xl px-4 pb-16 pt-8 md:px-6">
      <SceneBreadcrumb slug={data.slug} sceneName={data.sceneName} />

      <header className="mt-2">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <SceneCityHeading city={data.city} state={data.state} />
          <SceneWindowNav slug={data.slug} current={data.window} />
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
          {/* The window NAMES itself before it dates itself. `/week` can lead
              with a bare range because a Monday-to-Sunday span is
              self-describing; "Fri Aug 21 – Sun Aug 23" is not obviously a
              weekend, and the segment the reader typed is the honest label. */}
          <p className="font-mono text-sm">
            {SCENE_WINDOW_LABEL[data.window]}
            {range && `   ·   ${range}`}
            {'   ·   '}
            {/* The count is what the page RENDERED. When the cap cut the list,
                "60 shows" would be a total the page did not verify, so the
                truncated case says so in words instead. */}
            {truncated
              ? `first ${rendered} shows`
              : `${rendered} ${rendered === 1 ? 'show' : 'shows'}`}
            {data.trackedVenues.length > 0 &&
              `   ·   ${data.trackedVenues.length} rooms tracked`}
          </p>
          <ShareButton
            path={sceneWindowHref(data.slug, data.window)}
            ariaLabel={`Share ${SCENE_WINDOW_LABEL[data.window].toLowerCase()}`}
          />
        </div>

        {/* Load-bearing, not filler: coverage is a curated slice (11 rooms in
            Chicago, not all of Chicago). A page that implied full city coverage
            would be false, and a local would notice immediately. The quiet body
            below says the same thing in its own words, so it is not repeated
            there. */}
        {!isQuiet && (
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
            Every show at the {data.city} rooms we track. Not a complete city listing —
            see the rooms below.
          </p>
        )}
      </header>

      <div className="mt-6 border-t-2 border-foreground" />

      {isQuiet ? (
        <QuietWindow data={data} />
      ) : (
        days.map((day, i) => (
          <DayGroup
            key={day.date}
            day={day}
            // Only the LAST rendered day can have been cut by the row cap.
            countIsPartial={truncated && i === days.length - 1}
          />
        ))
      )}

      {/* A capped window would otherwise be a dead end: it is the widest thing
          the family offers, so none of the chips above lead anywhere that holds
          the rows it dropped. Stated as what the page DID show — "60 shows"
          alone would read as the window's total, which is the one number this
          page cannot verify. */}
      {truncated && (
        <p className="mt-6 border-t border-border pt-3 font-mono text-[11px] text-muted-foreground">
          Showing the first {rendered}.{' '}
          <Link
            href={allUpcomingHref(data.city, data.state)}
            className="underline underline-offset-4"
          >
            All upcoming in {data.city} →
          </Link>
        </p>
      )}

      {!isQuiet && <TrackedRoomsFooter city={data.city} rooms={data.trackedVenues} />}
    </div>
  )
}
