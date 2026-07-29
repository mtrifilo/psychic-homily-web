import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import {
  SCENE_WEEK_OG_ALT,
  renderSceneWeekOgCard,
} from '@/features/scenes/sceneWeekOgCard'

export const runtime = 'edge'
export const alt = SCENE_WEEK_OG_ALT
export const size = OG_SIZE
export const contentType = OG_CONTENT_TYPE

interface ImageProps {
  params: Promise<{ slug: string; 'iso-week': string }>
}

/**
 * Share card for `/scenes/{slug}/{iso-week}` — the archived permalink, and the
 * canonical URL for both weekly routes.
 *
 * This segment is dynamic, so it also catches any unmatched child path under a
 * scene; the week key is validated inside `renderSceneWeekOgCard`, which sends
 * anything that is not week-shaped to the fallback card.
 */
export default async function Image({ params }: ImageProps) {
  const { slug, 'iso-week': week } = await params
  return renderSceneWeekOgCard(slug, week)
}
