import {
  generateBreadcrumbSchema,
  generateItemListSchema,
  type BreadcrumbListSchema,
  type ItemListSchema,
  type MusicEventSchema,
} from '@/lib/seo/jsonld'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { formatWeekRange, showDisplayTitle, showHref, type SceneWeekResponse } from './sceneWeek'
import { sceneShowEvents } from './sceneShowJsonLd'

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
    events: sceneShowEvents(shows, week.timezone, now),
  }
}
