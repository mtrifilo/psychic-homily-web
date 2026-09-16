import type { Metadata } from 'next'
import {
  buildCitiesParam,
  cityKey,
  cityLabel,
  parseCitiesParam,
} from '@/components/filters/cityParams'
import type { CityState } from '@/components/filters'
import { clampPage, MAX_ARCHIVE_PAGE } from '@/features/shows/showArchive'
import { VENUES_ROOT } from '@/features/venues/venuesListNavigation'
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
 * Resolve `?cities=` against the facet.
 *
 * The facet's spelling wins over the URL's, matched case-insensitively, for the
 * same reason `VenueList` resolves its heading that way: `?cities=` is free text
 * off the URL, and it reaches the `<title>`, the description and the canonical.
 * Taking the matched row's spelling means a hand-crafted link cannot put
 * arbitrary text into any of the three.
 *
 * A null `facet` means the facet could not be read; see VenuesScopeKind.
 */
export function resolveVenuesScope(
  citiesParam: string | undefined,
  facet: FacetCity[] | null
): VenuesScope {
  const selected = parseCitiesParam(citiesParam)
  if (selected.length !== 1) return { kind: 'generic', city: null }
  if (!facet) return { kind: 'generic', city: null }

  const wanted = cityKey(selected[0]).toLowerCase()
  const match = facet.find(c => cityKey(c).toLowerCase() === wanted)
  if (!match) return { kind: 'unknown', city: null }
  return { kind: 'city', city: { city: match.city, state: match.state } }
}

/**
 * The page in view, from `?page=`.
 *
 * Clamped the way `VenueList` clamps it, so the canonical names the page the
 * reader is actually on rather than the number they typed. A value that is not
 * a number at all is page one.
 */
export function resolveVenuesPage(pageParam: string | undefined): number {
  const parsed = Number.parseInt(pageParam ?? '', 10)
  if (!Number.isFinite(parsed)) return 1
  return clampPage(parsed, MAX_ARCHIVE_PAGE)
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
  if (scope.kind === 'city' && scope.city) {
    const label = cityLabel(scope.city)
    const title = `Venues in ${label}`
    const description = `Live-music rooms in ${label}: upcoming shows, quiet rooms, links.`
    const canonical = venuesCityCanonical(
      VENUES_ROOT,
      buildCitiesParam([scope.city]),
      page
    )
    return {
      title,
      description,
      alternates: { canonical },
      openGraph: {
        title: `${title} | Psychic Homily`,
        description,
        url: canonical,
        type: 'website',
      },
    }
  }

  const canonical = listRootCanonical(VENUES_ROOT)
  return {
    title: VENUES_GENERIC_TITLE,
    description: VENUES_GENERIC_DESCRIPTION,
    alternates: { canonical },
    // A city with no verified rooms is a real page with nothing on it: it stays
    // reachable and its links stay followable, and it asks not to be indexed.
    // Every other generic state is the directory itself, which is indexable.
    ...(scope.kind === 'unknown'
      ? { robots: { index: false, follow: true } }
      : {}),
    openGraph: {
      title: `${VENUES_GENERIC_TITLE} | Psychic Homily`,
      description: VENUES_GENERIC_DESCRIPTION,
      url: VENUES_ROOT,
      type: 'website',
    },
  }
}

/** The first value of a Next search param, which may arrive as a list. */
export function firstParam(
  value: string | string[] | undefined
): string | undefined {
  return Array.isArray(value) ? value[0] : value
}
