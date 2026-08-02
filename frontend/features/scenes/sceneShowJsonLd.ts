import { generateMusicEventSchema, type MusicEventSchema } from '@/lib/seo/jsonld'
import { hasShowStarted } from '@/lib/utils/showTiming'
import type { SceneWeekShow } from './sceneWeek'

/**
 * The builder's input shape, pinned so this module's mapper is checked against
 * it. Without an explicit return type the mapper's shape is merely INFERRED,
 * and TypeScript skips its excess-property check on an inferred object handed
 * over as a function result. A key this builder no longer reads would then
 * compile clean here and surface only as a wrong payload: an offer left
 * standing for a show already underway. The page-level call site passes an
 * object literal, which the compiler does check; this pins the same guarantee
 * on the path that literal syntax does not cover.
 */
type MusicEventInput = Parameters<typeof generateMusicEventSchema>[0]

/**
 * Turning one scene show row into one `MusicEvent`.
 *
 * Lives apart from either page's JSON-LD builder because BOTH publish the same
 * shows: the weekly city page and the nightly one draw from one payload shape,
 * and a show described differently on the two would be a contradiction in the
 * structured data a crawler reads for the same URL.
 */

/**
 * The start instant, or `null` when the payload cannot supply one.
 *
 * `starts_at` is typed as required, but a TYPE is not a runtime guarantee: the
 * frontend and backend deploy separately, and Next's data cache can serve a
 * body fetched before the backend widened. `new Date(undefined)` is an invalid
 * date, and `Intl.DateTimeFormat.formatToParts` THROWS on one — from a server
 * component, which turns a missing structured-data field into a 500 for the
 * whole page. Parse defensively at this boundary.
 *
 * Exported because the nightly page renders a start TIME from the same field
 * and must apply the same rule: a second copy would eventually drift, and the
 * drift would show as a row displaying a time for a show the structured data
 * refuses to describe.
 */
export function startInstant(show: SceneWeekShow): string | null {
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
export function isDescribableEvent(show: SceneWeekShow): boolean {
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
 * payload lists names only, and inventing entity URLs from names is how
 * structured data starts lying. A show with no bill gets no `performer` for the
 * same reason (Google marks it recommended, not required).
 *
 * `sceneTimezone` is preferred over the state map as the fallback zone: the
 * backend bucketed this show into its day using the scene's modal venue zone,
 * and a venue whose own zone is missing must not be rendered against a
 * different one, and that is how the JSON-LD date ends up disagreeing with the
 * date heading above it. The state map is still the LAST resort underneath
 * both, inside `resolveShowTimezone`, shared with `generateMusicEventSchema`.
 */
function toMusicEventInput(
  show: SceneWeekShow,
  sceneTimezone: string | undefined,
  now: Date
): MusicEventInput {
  const startsAt = startInstant(show) as string // isDescribableEvent gated this

  const venue = {
    name: show.venue_name as string, // isDescribableEvent gated this
    slug: show.venue_slug || undefined,
    address: show.venue_address || undefined,
    city: show.venue_city || undefined,
    state: show.venue_state || undefined,
    country: show.venue_country || undefined,
    timezone: show.venue_timezone || sceneTimezone || undefined,
  }

  return {
    name: show.title || undefined,
    date: startsAt,
    is_cancelled: show.is_cancelled,
    is_sold_out: show.is_sold_out,
    // The archive goes back years, and an offer is a claim about what a reader
    // can still buy. Without this every archived page would advertise tickets
    // on sale for shows that are long over.
    //
    // The START INSTANT, deliberately, not the venue-local day: doors close at
    // a moment, and the shared module's `isShowPast` answers a different
    // question (how long the listing stays live) that would keep this offer
    // standing through the show itself.
    has_started: hasShowStarted(startsAt, now),
    venue,
    artists:
      show.artist_names && show.artist_names.length > 0
        ? show.artist_names.map(name => ({ name }))
        : undefined,
    price: show.price ?? undefined,
    slug: show.slug,
  }
}

/**
 * Every show that can be described, as `MusicEvent` schemas.
 *
 * May be shorter than the list it was given — a show without a venue or a start
 * instant is listed on the page but not published as an event.
 */
export function sceneShowEvents(
  shows: SceneWeekShow[],
  sceneTimezone: string | undefined,
  now: Date
): MusicEventSchema[] {
  return shows
    .filter(isDescribableEvent)
    .map(show => generateMusicEventSchema(toMusicEventInput(show, sceneTimezone, now)))
}
