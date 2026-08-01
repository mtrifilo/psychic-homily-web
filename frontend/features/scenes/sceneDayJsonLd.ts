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
  const shows = dayShows(day)
  const dateLabel = formatDayFull(day.date)

  // The dated permalink is the canonical URL for BOTH day routes (see
  // buildSceneDayMetadata), so the breadcrumb's leaf must point there too —
  // pointing it at /tonight would contradict the canonical tag, and would go
  // stale tomorrow besides.
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
