import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { fetchSceneDay } from '@/features/scenes/sceneDayApi'
import { renderSceneWeekOgCard } from '@/features/scenes/sceneWeekOgCard'
import { SCENE_WEEK_OG_ALT } from '@/features/scenes/sceneWeekOgLayout'
import { looksLikeISOWeek } from '@/features/scenes/sceneWeek'

export const runtime = 'edge'
export const alt = SCENE_WEEK_OG_ALT
export const size = OG_SIZE
export const contentType = OG_CONTENT_TYPE

interface ImageProps {
  params: Promise<{ slug: string }>
}

/**
 * Share card for `/scenes/{slug}/tonight` — the rolling, always-current night.
 *
 * The family has one renderer (`renderSceneWeekOgCard`); there is no separate
 * day card. Tonight is resolved the same way the page is: the backend names
 * the scene's current night (timezone + 6am boundary), and this route renders
 * that night's week. Calling the renderer with no week key would ask for the
 * CURRENT week, which disagrees with tonight on Monday before 6am.
 *
 * The rolling PAGE does not point its `og:image` here. This URL is a hash of
 * the route source, so it is fixed while the night changes, and third-party
 * unfurl caches key on the URL. `buildSceneDayMetadata` points the rolling
 * route at the archived week card instead, whose URL carries the night's
 * `iso_week`. This route stays because the card is legitimately addressable
 * on its own.
 */
export default async function Image({ params }: ImageProps) {
  const { slug } = await params
  const day = await fetchSceneDay(slug)
  const week =
    day?.iso_week && looksLikeISOWeek(day.iso_week) ? day.iso_week : undefined
  return renderSceneWeekOgCard(slug, week)
}
