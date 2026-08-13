import type { Metadata } from 'next'
import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'

/**
 * The archived-week card URL the rolling scene page advertises.
 *
 * A rolling URL's own `opengraph-image` never changes (Next hashes the route
 * source, not the week), and third-party unfurl caches key on that URL far
 * longer than any Cache-Control we set. Pointing `openGraph.images` at the
 * archived permalink makes a new week a new image. Same rule as
 * `buildSceneWeekMetadata`.
 */
export function sceneDetailOgImageUrl(slug: string, isoWeek: string): string {
  return `${SITE_URL}/scenes/${slug}/${isoWeek}/opengraph-image`
}

/**
 * The `openGraph.images` descriptor for the scene detail route.
 *
 * Setting `images` explicitly suppresses the file convention, so width / height
 * / type / alt have to be supplied here. Callers must OMIT `twitter.images` so
 * Next copies this full descriptor across rather than dropping the alt.
 */
export function sceneDetailOgImages(
  slug: string,
  isoWeek: string,
  alt: string
): NonNullable<NonNullable<Metadata['openGraph']>['images']> {
  return [
    {
      url: sceneDetailOgImageUrl(slug, isoWeek),
      width: OG_SIZE.width,
      height: OG_SIZE.height,
      type: OG_CONTENT_TYPE,
      alt,
    },
  ]
}
