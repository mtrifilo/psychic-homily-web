import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { renderSceneWeekOgCard } from '@/features/scenes/sceneWeekOgCard'
import { SCENE_WEEK_OG_ALT } from '@/features/scenes/sceneWeekOgLayout'

export const runtime = 'edge'
export const alt = SCENE_WEEK_OG_ALT
export const size = OG_SIZE
export const contentType = OG_CONTENT_TYPE

interface ImageProps {
  params: Promise<{ slug: string; period: string }>
}

/**
 * Share card for `/scenes/{slug}/{iso-week}` — the archived weekly permalink,
 * and the canonical URL for both weekly routes.
 *
 * This segment is dynamic, so it also catches any unmatched child path under a
 * scene; the week key is validated inside `renderSceneWeekOgCard`, which 404s
 * anything that is not week-shaped rather than paying for a card.
 *
 * That includes the DATED day permalink the segment now also serves. The day
 * page has no card of its own and does not advertise one — it sets
 * `openGraph.images` explicitly, so nothing ever links here for a date. A card
 * per night would be a second visual treatment for a page that deliberately
 * introduces none.
 */
export default async function Image({ params }: ImageProps) {
  const { slug, period } = await params
  return renderSceneWeekOgCard(slug, period)
}
