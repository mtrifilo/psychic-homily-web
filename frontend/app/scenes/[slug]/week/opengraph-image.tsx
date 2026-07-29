import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { renderSceneWeekOgCard } from '@/features/scenes/sceneWeekOgCard'
import { SCENE_WEEK_OG_ALT } from '@/features/scenes/sceneWeekOgLayout'

export const runtime = 'edge'
export const alt = SCENE_WEEK_OG_ALT
export const size = OG_SIZE
export const contentType = OG_CONTENT_TYPE

interface ImageProps {
  params: Promise<{ slug: string }>
}

/**
 * Share card for `/scenes/{slug}/week` — the rolling, always-current week.
 *
 * Note that the rolling PAGE does not point its `og:image` here: this URL is
 * fixed while its content changes weekly, and third-party unfurl caches key on
 * the URL, so they would serve whichever week they first saw. `buildSceneWeekMetadata`
 * points both routes at the archived card instead, whose URL carries the week.
 * This route stays because the card is legitimately addressable on its own.
 */
export default async function Image({ params }: ImageProps) {
  const { slug } = await params
  return renderSceneWeekOgCard(slug)
}
