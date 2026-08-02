import {
  formatDayHeading,
  formatShowCountLine,
  formatWeekRangeCompact,
} from '../sceneWeek'
import type { SceneListItem } from '../types'

/**
 * Rank order, ties broken by the order the API already chose.
 *
 * `sort` is stable, so scenes level on `shows_calendar_week` keep the list
 * endpoint's own ordering (total shows, then upcoming) rather than an arbitrary
 * one. That is what makes a quiet week's block deterministic: eleven scenes on
 * the long tail of scenes sitting on a count of 0 or 1 would otherwise be free
 * to reshuffle between renders.
 *
 * Zero-show scenes sort last for free and are NOT dropped. Hiding them would
 * make the block's membership churn week to week, and every row here is an
 * inbound link to a page that has no other one (PSY-1623) — a page linked only
 * in the weeks it happens to be busy is a page a crawler cannot rely on.
 */
function rankByWeek(scenes: readonly SceneListItem[]): SceneListItem[] {
  return [...scenes].sort((a, b) => b.shows_calendar_week - a.shows_calendar_week)
}

function CityRow({ scene }: { scene: SceneListItem }) {
  const quiet = scene.shows_calendar_week === 0

  return (
    // A plain anchor, not `next/link`. The locked decision for this block is
    // that it carries no client JS, and a bare anchor per scene is what
    // satisfies it: a `<Link>` per row would pull the router into a footer
    // index whose whole job is to be followed once, by a crawler. The cost is a
    // full navigation instead of a client-side one, which is why every other
    // internal link in this feature still uses `next/link`.
    <a
      href={`/scenes/${scene.slug}/week`}
      aria-label={`${scene.city}, ${scene.state}, ${formatShowCountLine(scene.shows_calendar_week, true)}`}
      className="group flex break-inside-avoid items-baseline gap-2 border-b border-border/50 py-2 transition-colors hover:bg-muted/40"
    >
      {/* `min-w-0` is what lets `truncate` actually truncate: a flex item
          defaults to `min-width: auto`, so without it a long city name pushes
          the count out of the row instead of ellipsizing. */}
      <span
        className={`min-w-0 truncate font-medium group-hover:underline ${
          quiet ? 'text-muted-foreground' : ''
        }`}
      >
        {scene.city}
      </span>
      <span className="shrink-0 font-mono text-[11px] text-muted-foreground">
        {scene.state}
      </span>
      <span className="ml-auto shrink-0 pl-2 font-mono text-sm text-muted-foreground">
        {scene.shows_calendar_week}
      </span>
    </a>
  )
}

/**
 * The by-city index that sits under the show list.
 *
 * WHAT THE COUNTS ARE, because the heading is looser than the numbers.
 * `shows_calendar_week` is each scene's OWN Monday-to-Sunday total, resolved in
 * its own venue timezone by `GET /scenes` — the same window, in the same zone,
 * that the page each row links to serves. A row's number therefore equals the
 * total on its destination, which is the whole point of the field existing:
 * `shows_this_week` is a ROLLING seven days from now and read 76 for Chicago
 * against that page's 96 on 2026-08-02.
 *
 * TWO THINGS STILL BLUR THAT EQUALITY, both bounded, neither hidden:
 *
 * 1. The RANGE beside the heading is ONE range for every row, derived in ONE
 *    zone (`SCENE_WEEK_INDEX_TIMEZONE`), while each scene resolves its own
 *    Monday. For the few hours a week between those Mondays a scene east of the
 *    heading's zone has already turned over while the label still names last
 *    week. Per-row ranges were not the approved design.
 *
 * 2. The counts arrive through the Data Cache while the heading reads the clock
 *    fresh, so at the rollover the numbers can lag the heading. `/shows` holds
 *    that to about a minute (`SCENE_WEEK_INDEX_REVALIDATE_SECONDS`); a caller
 *    that renders this block off an hour-cached payload would be wrong by an
 *    hour instead, so pass a payload as fresh as the heading.
 *
 * Column-major by CSS, deliberately: `columns-*` lays a single rank-ordered
 * list down column 1 then column 2, so the DOM order stays the rank order for a
 * crawler and a screen reader while the eye reads it in columns. Chunking into
 * per-column arrays would have fixed the column count on the server and lost
 * the 4-to-2 reflow.
 *
 * One row per scene, always. `GET /scenes` is unpaginated and bounded by the
 * scene thresholds, so the block grows with the catalogue, not with traffic.
 */
export function ThisWeekByCity({
  scenes,
  weekStart,
  weekEnd,
}: {
  scenes: readonly SceneListItem[]
  weekStart: string
  weekEnd: string
}) {
  if (scenes.length === 0) return null

  return (
    <section aria-labelledby="this-week-by-city" className="mt-12">
      <div className="border-t-2 border-foreground" />
      <div className="flex items-baseline justify-between gap-4 pt-4">
        <h2
          id="this-week-by-city"
          className="font-mono text-[11px] tracking-widest text-muted-foreground"
        >
          THIS WEEK, BY CITY
        </h2>
        {/* Two spellings of one range, swapped by CSS so neither needs JS: the
            weekdays are what make the range scannable, and they are the first
            thing to give at 390px. */}
        <span className="font-mono text-[11px] tracking-widest text-muted-foreground">
          <span className="sm:hidden">
            {formatWeekRangeCompact(weekStart, weekEnd)}
          </span>
          <span className="hidden sm:inline">
            {formatDayHeading(weekStart)} – {formatDayHeading(weekEnd)}
          </span>
        </span>
      </div>
      <div className="mt-2 columns-2 gap-x-8 md:columns-4 md:gap-x-10">
        {rankByWeek(scenes).map(scene => (
          <CityRow key={scene.slug} scene={scene} />
        ))}
      </div>
    </section>
  )
}
