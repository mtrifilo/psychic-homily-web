import {
  generateBreadcrumbSchema,
  generateItemListSchema,
  type BreadcrumbListSchema,
  type ItemListSchema,
  type MusicEventSchema,
} from '@/lib/seo/jsonld'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { showDisplayTitle, showHref } from './sceneWeek'
import { dayShows } from './sceneDay'
import { formatWindowRange } from './sceneWindow'
import { sceneShowEvents } from './sceneShowJsonLd'
import type { SceneSliceData } from './sceneSlice'

export interface SceneSliceJsonLd {
  breadcrumb: BreadcrumbListSchema
  /** Absent when the slice is quiet. An ItemList of nothing says nothing. */
  itemList?: ItemListSchema
  /** Emit as ONE array-valued script, not N tags. May be shorter than the list. */
  events: MusicEventSchema[]
}

/**
 * Every structured-data block the scene ROOT publishes.
 *
 * The root's structured data lists exactly the shows the root renders: this
 * builder reads the same slice payload `SceneCalendar` draws its rows from, so
 * the ItemList and the visible rows are one membership. `/tonight`,
 * `/this-weekend`, `/week` and `/next-4-weeks` each describe their own window,
 * so a crawler reaches every window through a page that shows it.
 *
 * Shows are taken in CLOCK order (`dayShows`), not the reader's live-night
 * order (`orderNightShows`), for the reason `buildSceneDayJsonLd` states about
 * the same night: an `ItemList` position outlives the hour that promoted a row.
 *
 * The scene's name and slug come from the day payload rather than the route
 * param, so a metro member (`/scenes/mesa-az`) is described under the canonical
 * scene it resolves to, exactly as the rendered links are.
 *
 * Returns null when the slice named no day, and when the day it named carries
 * no slug: an empty slug interpolates to `/scenes/`, which is the scenes INDEX
 * rather than a missing page, so a scene that cannot name its own URL publishes
 * nothing instead of pointing a crawler at a different page.
 *
 * `now` is injected so the past-show rule is testable without a fake clock.
 */
export function buildSceneSliceJsonLd(
  slice: SceneSliceData,
  now: Date = new Date()
): SceneSliceJsonLd | null {
  const [first] = slice.days
  if (!first || !first.slug) return null

  const shows = slice.days.flatMap(dayShows)

  // The trail terminates at the scene root because the root is not inside a
  // window: a leaf naming a week or a night would name a page with different
  // content. It names the scene the payload resolved to, which on a metro
  // member URL is not the URL the reader typed, and it is not this route's
  // `alternates.canonical` either. That is the same rule `buildSceneDayJsonLd`
  // states: a breadcrumb trail is a location, not a canonical claim.
  const breadcrumb = generateBreadcrumbSchema([
    { name: 'Home', url: SITE_URL },
    { name: 'Scenes', url: `${SITE_URL}/scenes` },
    { name: first.scene_name, url: `${SITE_URL}/scenes/${first.slug}` },
  ])

  if (shows.length === 0) {
    return { breadcrumb, events: [] }
  }

  const range = formatWindowRange(slice.days)

  return {
    breadcrumb,
    itemList: generateItemListSchema({
      name: range ? `${first.scene_name} shows, ${range}` : `${first.scene_name} shows`,
      listItems: shows.map(s => ({
        // Same helper the rendered rows link through, so a crawler's list and a
        // reader's list can never point at different URLs for one show.
        url: `${SITE_URL}${showHref(s)}`,
        name: showDisplayTitle(s),
      })),
    }),
    events: sceneShowEvents(shows, slice.timezone, now),
  }
}
