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
 * the ItemList and the visible rows are one membership. The wider windows are
 * described by their own routes. `/tonight`, `/this-weekend`, `/week` and
 * `/next-4-weeks` each publish an ItemList plus `MusicEvent[]` for the window
 * they render, so a crawler reaches every window through a page that shows it.
 *
 * Shows are taken in CLOCK order (`dayShows`), not the reader's live-night
 * order (`orderNightShows`): an `ItemList` position is a durable claim about a
 * cached and crawled page, while the live-night promotion changes with the
 * hour. The URLs and the membership are identical either way, and `/tonight`
 * publishes the same night under the same rule.
 *
 * The scene's name and slug come from the day payload rather than the route
 * param, so a metro member (`/scenes/mesa-az`) is described under the canonical
 * scene it resolves to, exactly as the rendered links are.
 *
 * Returns null when the slice named no day: without a payload there is no
 * scene name or canonical slug to describe, and a breadcrumb built from the
 * requested spelling would be a URL nothing else on the page points at.
 *
 * `now` is injected so the past-show rule is testable without a fake clock.
 */
export function buildSceneSliceJsonLd(
  slice: SceneSliceData,
  now: Date = new Date()
): SceneSliceJsonLd | null {
  const [first] = slice.days
  if (!first) return null

  const shows = slice.days.flatMap(dayShows)

  // The leaf is the root itself, which is also this page's canonical. The trail
  // terminates here because the root is not inside a window: pointing the leaf
  // at a week or a night would name a page with different content.
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
