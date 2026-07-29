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
  params: Promise<{ slug: string }>
}

/** Share card for `/scenes/{slug}/week` — the rolling, always-current week. */
export default async function Image({ params }: ImageProps) {
  const { slug } = await params
  return renderSceneWeekOgCard(slug)
}
