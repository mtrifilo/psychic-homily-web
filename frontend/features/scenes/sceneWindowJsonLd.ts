import {
  generateBreadcrumbSchema,
  generateItemListSchema,
  type BreadcrumbListSchema,
  type ItemListSchema,
  type MusicEventSchema,
} from '@/lib/seo/jsonld'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { showDisplayTitle, showHref } from './sceneWeek'
import { sceneShowEvents } from './sceneShowJsonLd'
import { SCENE_WINDOW_LABEL, formatWindowRange, sceneWindowHref } from './sceneWindow'
import type { SceneWindowData } from './components/SceneWindowView'

export interface SceneWindowJsonLd {
  breadcrumb: BreadcrumbListSchema
  /** Absent when the window is empty — an ItemList of nothing says nothing. */
  itemList?: ItemListSchema
  /** Emit as ONE array-valued script, not N tags. May be shorter than the list. */
  events: MusicEventSchema[]
}

/**
 * Every structured-data block a multi-week window page publishes.
 *
 * Mirrors `buildSceneWeekJsonLd` deliberately, down to emitting the events as a
 * single array: the two surfaces list the same shows in the same shape, and a
 * crawler that saw both must not be able to find them described differently.
 *
 * The breadcrumb leaf points at THIS window's own URL, unlike the week's, which
 * points at its archived permalink. These windows are rolling and have no
 * permalink to defer to — the segment is the only address they have.
 *
 * `now` is injected so the past-show rule is testable without a fake clock.
 */
export function buildSceneWindowJsonLd(
  data: SceneWindowData,
  now: Date = new Date()
): SceneWindowJsonLd {
  const label = SCENE_WINDOW_LABEL[data.window]
  const range = formatWindowRange(data.days)
  const shows = data.days.flatMap(d => d.shows ?? [])
  const name = range
    ? `${data.sceneName} shows, ${label.toLowerCase()} (${range})`
    : `${data.sceneName} shows, ${label.toLowerCase()}`

  const breadcrumb = generateBreadcrumbSchema([
    { name: 'Home', url: SITE_URL },
    { name: 'Scenes', url: `${SITE_URL}/scenes` },
    { name: data.sceneName, url: `${SITE_URL}/scenes/${data.slug}` },
    { name: label, url: `${SITE_URL}${sceneWindowHref(data.slug, data.window)}` },
  ])

  if (shows.length === 0) {
    return { breadcrumb, events: [] }
  }

  return {
    breadcrumb,
    itemList: generateItemListSchema({
      name,
      listItems: shows.map(s => ({
        // Same helper the rendered rows link through, so a crawler's list and a
        // reader's list can never point at different URLs for one show.
        url: `${SITE_URL}${showHref(s)}`,
        name: showDisplayTitle(s),
      })),
    }),
    events: sceneShowEvents(shows, data.timezone, now),
  }
}
