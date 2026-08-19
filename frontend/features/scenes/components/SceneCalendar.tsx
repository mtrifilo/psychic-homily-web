import Link from 'next/link'
// Deep-imported, not through `@/components/shared`: that barrel is reachable
// from the root layout, so importing a name through it is the habit that puts
// unrelated modules in the chunk every route loads (PSY-1772).
import { BracketLink } from '@/components/shared/BracketLink'
import { showDisplayTitle, showHref } from '../sceneWeek'
import {
  dayShows,
  formatDayCountLine,
  formatShowPrice,
  formatShowStartTime,
  type SceneDayResponse,
} from '../sceneDay'
import { formatSliceDateHeading, rowTimeZone, venueSubLocality } from '../sceneCalendar'
import {
  SCENE_WINDOW_LABEL,
  SCENE_WINDOW_ORDER,
  allUpcomingHref,
  sceneWindowHref,
} from '../sceneWindow'
import { sceneSliceIsQuiet, type SceneSliceData } from '../sceneSlice'
import { ShowStatusBadge } from './sceneChrome'
import type { SceneDetail, SceneShowSummary } from '../types'

/**
 * The scene root's calendar SLICE: tonight and the next full day, and the way
 * out to everything longer.
 *
 * The root used to BE the calendar — a 28-day window capped at 60 rows, which
 * put Chicago's first identity section 7.3 screens down at 390px. PSY-1850
 * inverts that (re-locking PSY-1729, Figma `1402-2`): the root is the front page
 * of the scene, an index into the place, and the window family
 * (`/tonight`, `/this-weekend`, `/week`, `/next-4-weeks`) is where a reader goes
 * for a calendar. The two differ in KIND, not in window length.
 *
 * A SERVER component, deliberately. It has no state and no interactivity, so
 * rendering it on the server ships its rows as HTML only — passing the same rows
 * through `SceneDetail`'s client boundary as props would duplicate every one of
 * them into the flight payload for nothing. `app/scenes/[slug]/page.tsx` renders
 * it and hands it to `SceneDetailView` as a slot.
 */

interface SceneCalendarProps {
  scene: SceneDetail
  /** Null when the day payload could not be fetched — NOT an empty calendar. */
  slice: SceneSliceData | null
}

/**
 * The window family, as a strip of path segments.
 *
 * ALL FOUR are links and NONE is active, which is the whole point of the
 * re-lock: the root is not one of these windows, so marking one of them current
 * would be false. (It used to draw `Next 4 weeks` as the active chip, because
 * the root's own window WAS four weeks. That window has moved to its own route.)
 *
 * Every window is a PATH SEGMENT, never a query param, and every href comes from
 * `sceneWindowHref` rather than being spelled here. That also retires a shipped
 * defect: this strip used to point `This weekend` and `This week` at the SAME
 * `/week` href, because `/this-weekend` did not exist when it was written. It
 * does now (PSY-1849).
 *
 * The strip never degrades. A thin scene renders the full row, because a window
 * that is empty is still an answer.
 */
function SceneWindowNav({ sceneSlug }: { sceneSlug: string }) {
  return (
    <nav
      aria-label="Show windows"
      className="flex flex-wrap items-center gap-x-8 gap-y-2 border-y border-border py-3"
    >
      {SCENE_WINDOW_ORDER.map(key => (
        <Link
          key={key}
          href={sceneWindowHref(sceneSlug, key)}
          className="rounded-sm font-mono text-[11px] uppercase tracking-widest text-primary transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"
        >
          {SCENE_WINDOW_LABEL[key]}
        </Link>
      ))}
    </nav>
  )
}

/**
 * One show: time, bill, price, room.
 *
 * The row grammar `/tonight` already proved, plus the sub-locality the prior art
 * puts on every row. Printing `(Mesa)` beside `(Phoenix)` is what lets a metro
 * scene read as a region for free, and it costs nothing because the payload
 * already carries `venue_city`.
 *
 * ONE outer link, to the show. The mock draws the room as its own link, but a
 * nested anchor is invalid HTML and a row with two targets is a row a reader has
 * to aim at; the room's own page is one hop away from the show.
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
  // default and put an Arizona clock under a UTC-derived date. An absent time is
  // a smaller loss than a confident wrong one, and the column below is
  // width-reserved either way.
  const zone = rowTimeZone(show) ?? sceneTimeZone
  const time = zone ? formatShowStartTime(show, zone) : null
  const price = formatShowPrice(show)
  const subLocality = venueSubLocality(show)

  return (
    <li className="border-b border-border/40 last:border-b-0">
      {/* TWO lines at mobile, not three, and this is what buys the fold.
          `flex-col` stacked the time onto a line of its own, which cost ~26px on
          EVERY row — about 450px across a dense scene's two nights, or half a
          screen of pure gutter. The mock puts the time in a left column beside
          the bill and drops the room underneath, so the grid does that: time and
          bill share row 1, the room spans row 2. At `sm` it collapses back to
          the single flex line the wider viewport can hold. */}
      <Link
        href={showHref(show)}
        className="group grid grid-cols-[auto_1fr] items-baseline gap-x-3 gap-y-0.5 py-2.5 transition-colors hover:bg-muted/40 sm:flex sm:items-baseline sm:gap-4"
      >
        {/* Width-reserved even when the instant is unusable, so the bills stay in
            one column rather than one row jumping left. */}
        <span className="col-start-1 row-start-1 shrink-0 font-mono text-xs tabular-nums text-muted-foreground sm:w-20">
          {time ?? ''}
        </span>
        <span className="col-start-2 row-start-1 flex min-w-0 flex-wrap items-center gap-2">
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
        {/* The price rides in the LEFT GUTTER under the time, so a priced row
            costs no more height than an unpriced one. Putting it on a line of
            its own would undo the saving this grid exists for. */}
        {price && (
          <span className="col-start-1 row-start-2 shrink-0 font-mono text-xs tabular-nums text-muted-foreground sm:row-start-auto">
            {price}
          </span>
        )}
        {show.venue_name && (
          <span className="col-start-2 row-start-2 min-w-0 font-mono text-xs text-muted-foreground sm:shrink-0">
            {show.venue_name}
            {subLocality ? ` ${subLocality}` : ''}
          </span>
        )}
      </Link>
    </li>
  )
}

/**
 * One whole date: heading, count and rows.
 *
 * The count is UNQUALIFIED here, unlike the old windowed calendar's, and that is
 * the payoff of reading the day endpoint: the backend answered for this entire
 * scene-local date, so `n shows` is that date's real total rather than however
 * many rows survived a window's row cap. Nothing on this surface has to suppress
 * a count it cannot stand behind any more.
 *
 * A date with nothing on it still renders its heading and its `0 shows listed`.
 * Showing the date is what tells a reader we checked it — and unlike the old
 * root's synthesized empty bucket, this one is drawn only for a date a payload
 * actually answered for.
 */
function SceneDateGroup({
  day,
  sceneTimeZone,
}: {
  day: SceneDayResponse
  sceneTimeZone?: string
}) {
  const shows = dayShows(day)

  return (
    <section className="border-t border-border pt-4">
      <div className="flex items-baseline justify-between gap-4">
        <h3 className="font-mono text-[11px] uppercase tracking-widest">
          {/* TONIGHT LEADS, per the mock: it is the word the reader is scanning
              for, and it comes from the backend's `is_tonight` against the
              scene's own 6am boundary — never from a clock on this side. */}
          {day.is_tonight && <span className="text-primary">TONIGHT </span>}
          {formatSliceDateHeading(day.date)}
        </h3>
        <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
          {formatDayCountLine(shows.length)}
        </span>
      </div>
      {shows.length > 0 && (
        <ul className="mt-1">
          {shows.map(show => (
            <SceneShowRow key={show.id} show={show} sceneTimeZone={sceneTimeZone} />
          ))}
        </ul>
      )}
    </section>
  )
}

/**
 * Where the rest of the calendar lives.
 *
 * The slice's terminator, and the locked mock's two buttons. It makes NO claim
 * about how much was shown or withheld: the old footer said "Showing everything
 * we have in the next four weeks", which this page is no longer entitled to say
 * and which the date headings above already answer for the two nights it does
 * cover.
 *
 * Deliberately NOT carrying share / .ics again. Both already sit in the header's
 * action row a screen above, and two links with the same name AND href in one
 * render give a reader using the links list two identical entries to choose
 * between.
 */
function SliceWaysOut({ scene }: { scene: SceneDetail }) {
  return (
    <>
      <BracketLink
        label={`Full week in ${scene.city} →`}
        href={sceneWindowHref(scene.slug, 'this-week')}
      />
      <BracketLink
        label="Next 4 weeks →"
        href={sceneWindowHref(scene.slug, 'next-4-weeks')}
      />
    </>
  )
}

function SliceFooter({ scene }: { scene: SceneDetail }) {
  return (
    <div className="mt-6 flex flex-wrap items-center gap-4 border-t border-border pt-3">
      <SliceWaysOut scene={scene} />
    </div>
  )
}

/**
 * The copy for a slice with nothing on it.
 *
 * Never asserts that no show exists, only that none is on OUR calendar.
 * Coverage is a curated slice of each city's rooms, so "no shows" would be a
 * claim about the city this site is not entitled to make, and a local who knows
 * better would be right to stop trusting the rest of the page. The same rule
 * `/tonight`'s quiet night already follows.
 *
 * `roomCount` is named because it is what makes the sentence checkable: a reader
 * who knows the city can see exactly how wide the claim is.
 *
 * The window is named as the two nights it actually covered, NOT as "the next
 * four weeks" (which the old root's copy claimed, and which this page no longer
 * looks at). A quiet slice says nothing whatever about the month.
 */
function quietSliceCopy(city: string, roomCount: number, dayCount: number): string {
  const rooms = roomCount > 0 ? `the ${roomCount} ${city} rooms we track` : `the ${city} rooms we track`
  const when = dayCount > 1 ? 'tonight or tomorrow' : 'tonight'
  return `Nothing on our calendar for ${rooms} ${when}. A room may have shows we have not listed.`
}

/**
 * Honest zero, plus exactly ONE step wider.
 *
 * The cheapest correct empty state in the prior art: state the zero, then offer
 * the next-widest view. No empty-state art, no "clear filters" dead end, and no
 * padding the window with other cities' shows.
 *
 * `All upcoming in {city}` rides here as the second step out, because the scene
 * that is quiet for two nights is exactly the scene whose WEEK may also be
 * quiet — offering only the week would risk handing that reader a second empty
 * page and no third door.
 */
function QuietSlice({ scene, dayCount }: { scene: SceneDetail; dayCount: number }) {
  return (
    <div className="border-t border-border py-4">
      <p className="max-w-2xl text-sm text-muted-foreground">
        {quietSliceCopy(scene.city, scene.stats.venue_count, dayCount)}
      </p>
      {/* The populated footer's two links FIRST, from the same component, so a
          reader who finds nothing here is offered the identical way out under
          the identical label — then the two extra doors a quiet slice needs. */}
      <div className="mt-3 flex flex-wrap items-center gap-4">
        <SliceWaysOut scene={scene} />
        <BracketLink
          label={`All upcoming in ${scene.city} →`}
          href={allUpcomingHref(scene.city, scene.state)}
        />
        <BracketLink label="Suggest a venue →" href="/contribute" />
      </div>
    </div>
  )
}

export function SceneCalendar({ scene, slice }: SceneCalendarProps) {
  return (
    <div>
      <SceneWindowNav sceneSlug={scene.slug} />

      {/* No section heading and no accuracy disclaimer, both per the locked mock
          (`1402-2`). The date headings below ARE the heading — a `Shows / next 4
          weeks` H2 over two nights was the false claim this ticket exists to
          retire, and there is no true version of it worth ~110px above the fold
          on the surface whose whole defect was depth. The coverage and accuracy
          voice lives on the window pages, which is where a reader who came for a
          calendar ends up. */}
      <div className="mt-6 space-y-5">
        {slice === null ? (
          /* A failed request is NOT an empty calendar. Falling through to the
             honest-zero copy would state, in our own voice and with a room count
             attached, that nothing is on tonight, a claim we did not check. Say
             what actually happened instead. */
          <p className="text-sm text-muted-foreground">
            We could not load this scene&apos;s calendar just now. Try again in a moment,
            or see{' '}
            <Link
              href={sceneWindowHref(scene.slug, 'this-week')}
              className="underline underline-offset-4"
            >
              the full week in {scene.city}
            </Link>
            .
          </p>
        ) : sceneSliceIsQuiet(slice) ? (
          <QuietSlice scene={scene} dayCount={slice.days.length} />
        ) : (
          <>
            {slice.days.map(day => (
              <SceneDateGroup key={day.date} day={day} sceneTimeZone={slice.timezone} />
            ))}
            <SliceFooter scene={scene} />
          </>
        )}
      </div>
    </div>
  )
}
