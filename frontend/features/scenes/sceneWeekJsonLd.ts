import {
  generateBreadcrumbSchema,
  generateItemListSchema,
  generateMusicEventSchema,
  type BreadcrumbListSchema,
  type ItemListSchema,
  type MusicEventSchema,
} from '@/lib/seo/jsonld'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import {
  formatWeekRange,
  showDisplayTitle,
  showHref,
  type SceneWeekResponse,
  type SceneWeekShow,
} from './sceneWeek'

/**
 * The start instant, or `null` when the payload cannot supply one.
 *
 * `starts_at` is typed as required, but a TYPE is not a runtime guarantee: the
 * frontend and backend deploy separately, and Next's data cache can serve a
 * body fetched before the backend widened. `new Date(undefined)` is an invalid
 * date, and `Intl.DateTimeFormat.formatToParts` THROWS on one — from a server
 * component, which turns a missing structured-data field into a 500 for the
 * whole page. Parse defensively at this boundary.
 */
function startInstant(show: SceneWeekShow): string | null {
  const raw = show.starts_at
  if (typeof raw !== 'string' || !Number.isFinite(Date.parse(raw))) return null
  return raw
}

/**
 * Whether a show can be described as an event at all.
 *
 * Google requires `name`, `startDate` and a `location` carrying an address; a
 * show missing its venue or its start instant cannot produce a valid one, and
 * publishing a half-event is worse than publishing none — the show is still
 * listed and linked by the ItemList either way.
 */
function isDescribableEvent(show: SceneWeekShow): boolean {
  return Boolean(show.venue_name) && startInstant(show) !== null
}

/**
 * One show as `MusicEvent` input.
 *
 * An empty `title` is forwarded as undefined so the shared generator composes
 * "<headliner> at <venue>", exactly as `/shows/{slug}` does for the same show.
 * Note the two pages can still land on different names: the detail payload
 * carries an explicit `is_headliner` and honours it at any bill position, while
 * this payload carries names only, so the generator falls back to the first.
 * They agree whenever the headliner leads the bill, which is the normal case.
 *
 * Artists come from `artist_names` in bill order, which is what the page itself
 * renders. They carry no slug, so performers are named but not linked — the
 * week payload lists names only, and inventing entity URLs from names is how
 * structured data starts lying. A show with no bill gets no `performer` for the
 * same reason (Google marks it recommended, not required).
 */
function toMusicEventInput(show: SceneWeekShow, week: SceneWeekResponse, now: Date) {
  const startsAt = startInstant(show) as string // isDescribableEvent gated this

  const venue = {
    name: show.venue_name as string, // isDescribableEvent gated this
    slug: show.venue_slug || undefined,
    address: show.venue_address || undefined,
    city: show.venue_city || undefined,
    state: show.venue_state || undefined,
    country: show.venue_country || undefined,
    // The scene's own zone is the fallback, NOT the state map: the backend
    // bucketed this show into its day using the scene's modal venue zone, and a
    // venue whose own zone is missing must not be rendered against a different
    // one — that is how the JSON-LD date ends up disagreeing with the day
    // heading directly above it.
    timezone: show.venue_timezone || week.timezone || undefined,
  }

  return {
    name: show.title || undefined,
    date: startsAt,
    is_cancelled: show.is_cancelled,
    is_sold_out: show.is_sold_out,
    // The archive goes back years, and an offer is a claim about what a reader
    // can still buy. Without this every archived week would advertise tickets
    // on sale for shows that are long over.
    is_past: Date.parse(startsAt) <= now.getTime(),
    venue,
    artists:
      show.artist_names && show.artist_names.length > 0
        ? show.artist_names.map(name => ({ name }))
        : undefined,
    price: show.price ?? undefined,
    slug: show.slug,
  }
}

export interface SceneWeekJsonLd {
  breadcrumb: BreadcrumbListSchema
  /** Absent when the week is empty — an ItemList of nothing says nothing. */
  itemList?: ItemListSchema
  /** Emit as ONE array-valued script, not N tags. May be shorter than the list. */
  events: MusicEventSchema[]
}

/**
 * Every structured-data block the weekly city page publishes.
 *
 * The page's visible text has always carried the date, the venue and the bill;
 * before this the machine-readable copy carried only show URLs and their bill
 * names, with no time, place, price or status. The events go through the SAME
 * generator `/shows/{slug}` uses, so the two surfaces cannot drift in how they
 * SHAPE an event — though their inputs are picked separately, so a multi-venue
 * show can still name a different room on each (this page takes the
 * alphabetically-first in-scope venue; the detail page takes whichever its
 * unordered preload returns first). `image` is a second such input difference:
 * `SceneShowSummary` carries no `image_url`, so this page's array is the
 * generated card alone where the detail page also lists the flyer.
 *
 * `now` is injected so the past-show rule is testable without a fake clock.
 *
 * Pure and page-agnostic so both routes — the rolling `/week` and the archived
 * `/{iso-week}` — get identical output for the same week, and so this can be
 * asserted on without rendering a server component.
 */
export function buildSceneWeekJsonLd(
  week: SceneWeekResponse,
  now: Date = new Date()
): SceneWeekJsonLd {
  const range = formatWeekRange(week.start_date, week.end_date)
  const shows = (week.days ?? []).flatMap(d => d.shows ?? [])

  // The archived permalink is the canonical URL for BOTH routes (see
  // buildSceneWeekMetadata), so the breadcrumb's leaf must point there too —
  // pointing it at the rolling URL would contradict the canonical tag.
  const breadcrumb = generateBreadcrumbSchema([
    { name: 'Home', url: SITE_URL },
    { name: 'Scenes', url: `${SITE_URL}/scenes` },
    { name: week.scene_name, url: `${SITE_URL}/scenes/${week.slug}` },
    { name: range, url: `${SITE_URL}/scenes/${week.slug}/${week.iso_week}` },
  ])

  if (shows.length === 0) {
    return { breadcrumb, events: [] }
  }

  return {
    breadcrumb,
    itemList: generateItemListSchema({
      name: `${week.scene_name} shows, ${range}`,
      listItems: shows.map(s => ({
        // Same helper the rendered rows link through, so a crawler's list and a
        // reader's list can never point at different URLs for one show.
        url: `${SITE_URL}${showHref(s)}`,
        name: showDisplayTitle(s),
      })),
    }),
    events: shows
      .filter(isDescribableEvent)
      .map(s => generateMusicEventSchema(toMusicEventInput(s, week, now))),
  }
}
