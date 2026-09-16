import type { Metadata } from 'next'
import {
  buildCitiesParam,
  cityLabel,
  parseCitiesParam,
} from '@/components/filters/cityParamsFormat'
import type { CityState } from '@/components/filters'
import { MAX_ARCHIVE_PAGE } from '@/features/shows/showArchive'
import { archivePageNumber } from '@/features/shows/showArchive.server'
import { facetCityFor, VENUES_ROOT } from '@/features/venues/venuesListNavigation'
import { listRootCanonical, venuesCityCanonical } from '@/lib/seo/siteMetadata'

/** The directory's title when it is not about one city. */
export const VENUES_GENERIC_TITLE = 'Venues'

/** The directory's description when it is not about one city. */
export const VENUES_GENERIC_DESCRIPTION =
  'Browse music venues and discover upcoming shows.'

/** One row of the `/venues/cities` facet, reduced to what naming a city needs. */
export interface FacetCity {
  city: string
  state: string
}

/**
 * What the URL asks this page to be about, decided against the city facet.
 *
 *   `city`    exactly one city, and the facet offers it.
 *   `unknown` exactly one city, and the facet does not offer it.
 *   `generic` no city, `?cities=all`, or more than one city.
 *
 * `unknown` is its own state rather than a flavour of `generic` because the two
 * differ in what a crawler should do: a city the facet does not offer has no
 * verified rooms, so the page is real but has nothing to index.
 *
 * The facet being UNAVAILABLE is deliberately `generic` rather than `unknown`.
 * Failing the other way would turn a backend blip into a noindex on every city
 * page for a whole cache window, which is a far more expensive mistake than an
 * indexable empty page for one.
 */
export type VenuesScopeKind = 'city' | 'unknown' | 'generic'

export interface VenuesScope {
  kind: VenuesScopeKind
  /** The facet's spelling of the city, present only for `city`. */
  city: CityState | null
}

/**
 * Resolve a selection against the facet.
 *
 * `facetCityFor` is the shared rule, so this page's `<title>` and its `<h1>`
 * cannot disagree about which city the URL names. What is decided HERE is only
 * what a miss MEANS to a crawler, which is a question the heading does not ask.
 *
 * A null `facet` means the facet could not be read; see VenuesScopeKind.
 */
export function resolveVenuesScope(
  selected: CityState[],
  facet: FacetCity[] | null
): VenuesScope {
  if (selected.length !== 1 || !facet) return { kind: 'generic', city: null }
  const city = facetCityFor(selected, facet)
  return city ? { kind: 'city', city } : { kind: 'unknown', city: null }
}

/** The cities `?cities=` names, in the wire format every surface shares. */
export function parseVenuesCities(citiesParam: string | undefined): CityState[] {
  return parseCitiesParam(citiesParam)
}

/**
 * The page in view, bounded the way the list bounds it, so the canonical names
 * the page the reader is actually on rather than the number they typed.
 *
 * Through the shared server-side derivation rather than a `parseInt` of its
 * own: the browser reads `?page=` with nuqs, and the two spell some values
 * differently (`archivePageNumber` records which).
 */
export function resolveVenuesPage(
  searchParams: Record<string, string | string[] | undefined>
): number {
  return archivePageNumber(searchParams, MAX_ARCHIVE_PAGE)
}

/**
 * The directory's metadata for one resolved state.
 *
 * Split out from `generateMetadata` so every state can be asserted without a
 * fetch, and so the three facts that move together — title, canonical and
 * robots — are decided in one place rather than in three conditionals.
 *
 * Open Graph carries the same title and description and the same URL as the
 * canonical: a shared city link and the indexed city page are one address.
 */
export function buildVenuesMetadata(
  scope: VenuesScope,
  page: number
): Metadata {
  const named = scope.kind === 'city' && scope.city ? cityLabel(scope.city) : null

  const title = named ? `Venues in ${named}` : VENUES_GENERIC_TITLE
  const description = named
    ? `Live-music rooms in ${named}: upcoming shows, quiet rooms, links.`
    : VENUES_GENERIC_DESCRIPTION
  const canonical =
    scope.kind === 'city' && scope.city
      ? venuesCityCanonical(VENUES_ROOT, buildCitiesParam([scope.city]), page)
      : listRootCanonical(VENUES_ROOT)

  return {
    title,
    description,
    alternates: { canonical },
    // A city with no verified rooms is a real page with nothing on it: it stays
    // reachable and its links stay followable, and it asks not to be indexed.
    // Every other state is the directory itself, which is indexable.
    ...(scope.kind === 'unknown'
      ? { robots: { index: false, follow: true } }
      : {}),
    // The same URL as the canonical: a shared link and the indexed page are one
    // address.
    openGraph: {
      title: `${title} | Psychic Homily`,
      description,
      url: canonical,
      type: 'website',
    },
  }
}

/**
 * The first value of a Next search param, which may arrive as a list.
 *
 * FIRST rather than "a repeat is invalid", because that is what the browser
 * does: `useSearchParams().get('cities')` returns the first value, so a
 * repeated parameter has to name the same page on both sides.
 */
export function firstParam(
  value: string | string[] | undefined
): string | undefined {
  return Array.isArray(value) ? value[0] : value
}
