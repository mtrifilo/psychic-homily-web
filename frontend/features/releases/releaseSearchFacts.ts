import {
  releaseLinkPlatformLabel,
  renderableReleaseLinks,
  type ReleaseLinkLike,
} from '@/lib/releaseLinks'
import { getReleaseTypeLabel } from './types'

/**
 * The facts a release page's search snippet and structured data state, read
 * from the release payload the page already renders.
 *
 * Every value here is one the page itself shows: the same artists, type label,
 * labels and link names, so the snippet cannot promise what the page lacks.
 * No directive and no client-only imports, because the route's server module
 * reads it.
 */

interface CreditedArtistLike {
  name: string
  role?: string | null
}

/**
 * The artists a release is "by": those with the `main` role, in payload order
 * (the backend's credit position). A release with no `main` credit is by its
 * first credited artist. Blank names are skipped. Empty when the release
 * credits no one.
 */
export function creditedReleaseArtists<T extends CreditedArtistLike>(
  artists: ReadonlyArray<T> | null | undefined
): T[] {
  const named = (artists ?? []).filter(artist => artist.name?.trim())
  const main = named.filter(artist => artist.role === 'main')
  if (main.length > 0) return main
  return named.slice(0, 1)
}

/**
 * Platforms the page lists under "Listen / Buy" that are reference databases,
 * not places to play or buy the music, so a "Listen:" clause never names them.
 * The link registry in `lib/releaseLinks` records no such distinction, so the
 * keys are listed here.
 */
const NOT_LISTENABLE_PLATFORMS: ReadonlySet<string> = new Set(['discogs'])

/**
 * The names of the services where the release can be played or bought, under
 * the labels the page prints, in page order, each once. Only links the page
 * renders count.
 */
export function releaseListenServiceNames(
  links: ReadonlyArray<ReleaseLinkLike> | null | undefined
): string[] {
  const names = renderableReleaseLinks(links)
    .filter(link => !NOT_LISTENABLE_PLATFORMS.has(link.platform.toLowerCase()))
    .map(link => releaseLinkPlatformLabel(link.platform))
  return [...new Set(names)]
}

/**
 * The first label the page lists that has a name, or null. The page lists
 * labels in payload order.
 */
export function releaseLabelName(
  labels: ReadonlyArray<{ name: string }> | null | undefined
): string | null {
  return labels?.find(label => label.name?.trim())?.name ?? null
}

/** The type label the page shows, or null when the stored type is empty. */
export function releaseTypeLabel(
  releaseType: string | null | undefined
): string | null {
  return releaseType?.trim() ? getReleaseTypeLabel(releaseType).trim() : null
}

/**
 * The release year when it is a usable calendar year, or null. A year of zero
 * or a fraction is not a publication date.
 */
export function knownReleaseYear(year: number | null | undefined): number | null {
  return typeof year === 'number' && Number.isInteger(year) && year > 0
    ? year
    : null
}
