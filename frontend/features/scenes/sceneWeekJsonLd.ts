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
 * One show as `MusicEvent` input.
 *
 * Deliberately shaped to match what `/shows/{slug}` passes for the SAME show:
 * an empty `title` is forwarded as undefined so the shared generator composes
 * "<headliner> at <venue>" identically on both pages. A crawler that sees the
 * same event on a listing page and its detail page must not be told two
 * different names for it.
 *
 * Artists come from `artist_names` in bill order, which is what the page itself
 * renders. They carry no slug, so performers are named but not linked — the
 * week payload lists names only, and inventing entity URLs from names is how
 * structured data starts lying.
 */
function toMusicEventInput(show: SceneWeekShow) {
  const venue = show.venue_name
    ? {
        name: show.venue_name,
        slug: show.venue_slug || undefined,
        address: show.venue_address || undefined,
        city: show.venue_city || undefined,
        state: show.venue_state || undefined,
        // `resolveShowTimezone` falls back to the state map when this is absent,
        // so a venue with no geocoded zone still gets a real local offset.
        timezone: show.venue_timezone || undefined,
      }
    : undefined

  const artists = (show.artist_names ?? []).map(name => ({ name }))

  return {
    name: show.title || undefined,
    date: show.starts_at,
    is_cancelled: show.is_cancelled,
    is_sold_out: show.is_sold_out,
    venue,
    artists: artists.length > 0 ? artists : undefined,
    price: show.price ?? undefined,
    slug: show.slug,
  }
}

export interface SceneWeekJsonLd {
  breadcrumb: BreadcrumbListSchema
  /** Absent when the week is empty — an ItemList of nothing says nothing. */
  itemList?: ItemListSchema
  /** Empty when the week is empty; emit as ONE array-valued script, not N tags. */
  events: MusicEventSchema[]
}

/**
 * Every structured-data block the weekly city page publishes.
 *
 * The page's visible text has always carried the date, the venue and the bill;
 * before this the machine-readable copy carried only 22 bare URLs, making the
 * most parseable surface on the page the least informative one. The events are
 * built by the SAME generator `/shows/{slug}` uses, so the two surfaces cannot
 * disagree about a show's start time, venue address or status.
 *
 * Pure and page-agnostic so both routes — the rolling `/week` and the archived
 * `/{iso-week}` — get byte-identical output for the same week, and so this can
 * be asserted on without rendering a server component.
 */
export function buildSceneWeekJsonLd(week: SceneWeekResponse): SceneWeekJsonLd {
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
    events: shows.map(s => generateMusicEventSchema(toMusicEventInput(s))),
  }
}
