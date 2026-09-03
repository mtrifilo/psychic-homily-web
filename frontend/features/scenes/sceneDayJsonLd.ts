import {
  generateBreadcrumbSchema,
  generateItemListSchema,
  type BreadcrumbListSchema,
  type ItemListSchema,
  type MusicEventSchema,
} from '@/lib/seo/jsonld'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { showDisplayTitle, showHref } from './sceneWeek'
import { dayShows, formatDayFull, type SceneDayResponse } from './sceneDay'
import { sceneShowEvents } from './sceneShowJsonLd'

export interface SceneDayJsonLd {
  breadcrumb: BreadcrumbListSchema
  /** Absent when the night is quiet — an ItemList of nothing says nothing. */
  itemList?: ItemListSchema
  /** Emit as ONE array-valued script, not N tags. May be shorter than the list. */
  events: MusicEventSchema[]
}

/**
 * Every structured-data block a scene's nightly page publishes.
 *
 * The events go through the same generator as the weekly page and
 * `/shows/{slug}`, via `sceneShowEvents`, so one show cannot be described
 * differently depending on which page a crawler reached it from.
 *
 * `now` is injected so the past-show rule is testable without a fake clock.
 *
 * Pure and page-agnostic so both routes — the rolling `/tonight` and the dated
 * permalink — get identical output for the same night, and so this can be
 * asserted on without rendering a server component.
 */
export function buildSceneDayJsonLd(
  day: SceneDayResponse,
  now: Date = new Date()
): SceneDayJsonLd {
  // Clock order, NOT the reader's live-night order (`orderNightShows`). An
  // `ItemList` position is a durable claim about a page that is cached and
  // crawled, and the live-night promotion changes with the hour: stamping it
  // here would bake one minute's ordering into structured data that outlives
  // it. The URLs and the membership are identical either way.
  const shows = dayShows(day)
  const dateLabel = formatDayFull(day.date)

  // The leaf names the DATED permalink from both day routes. It is the only URL
  // that identifies this night: /tonight would go stale tomorrow, and the week
  // permalink the rolling route declares as its canonical (see
  // buildSceneDayMetadata) names seven nights rather than this one. A
  // breadcrumb trail is a location, not a canonical claim.
  const breadcrumb = generateBreadcrumbSchema([
    { name: 'Home', url: SITE_URL },
    { name: 'Scenes', url: `${SITE_URL}/scenes` },
    { name: day.scene_name, url: `${SITE_URL}/scenes/${day.slug}` },
    { name: dateLabel, url: `${SITE_URL}/scenes/${day.slug}/${day.date}` },
  ])

  if (shows.length === 0) {
    return { breadcrumb, events: [] }
  }

  return {
    breadcrumb,
    itemList: generateItemListSchema({
      name: `${day.scene_name} shows, ${dateLabel}`,
      listItems: shows.map(s => ({
        // Same helper the rendered rows link through, so a crawler's list and a
        // reader's list can never point at different URLs for one show.
        url: `${SITE_URL}${showHref(s)}`,
        name: showDisplayTitle(s),
      })),
    }),
    events: sceneShowEvents(shows, day.timezone, now),
  }
}
