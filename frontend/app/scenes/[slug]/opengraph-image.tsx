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
 * Share card for `/scenes/{slug}` — the rolling scene detail URL.
 *
 * The PAGE does not point its `og:image` here. This URL is fixed while the
 * week it would draw changes, and third-party unfurl caches key on the URL, so
 * they would serve whichever week they first saw. `generateMetadata` points
 * the detail route at the archived week card instead, whose URL carries the
 * week. This route stays because the card is legitimately addressable on its
 * own, matching `/scenes/{slug}/week/opengraph-image`.
 *
 * Reuses `renderSceneWeekOgCard` (locked P5). Do not add fonts: the OG edge
 * family is already at 96.5% of Vercel's 1MB cap. Do not import via a barrel
 * that pulls `lib/og/response` into a non-OG route.
 */
export default async function Image({ params }: ImageProps) {
  const { slug } = await params
  return renderSceneWeekOgCard(slug)
}
