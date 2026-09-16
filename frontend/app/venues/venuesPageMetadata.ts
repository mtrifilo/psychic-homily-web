import type { Metadata } from 'next'
import {
  buildCitiesParam,
  cityLabel,
  parseCitiesParam,
} from '@/components/filters/cityParamsFormat'
import type { CityState } from '@/components/filters'
import { MAX_ARCHIVE_PAGE } from '@/features/shows/showArchive'
import { archivePageNumber } from '@/features/shows/showArchive.server'
import {
  facetCityFor,
  VENUES_PAGE_SIZE,
  VENUES_ROOT,
} from '@/features/venues/venuesListNavigation'
import { listRootCanonical, venuesCityCanonical } from '@/lib/seo/siteMetadata'

/** The directory's title when it is not about one city. */
export const VENUES_GENERIC_TITLE = 'Venues'

/** The directory's description when it is not about one city. */
export const VENUES_GENERIC_DESCRIPTION =
  'Browse music venues and discover upcoming shows.'

/**
 * One row of the `/venues/cities` facet, reduced to what this page needs: the
 * spelling to name the city by, and the room count, which is also the size of
 * the city's page space.
 */
export interface FacetCity {
  city: string
  state: string
  venue_count: number
}

/**
 * What the URL asks this page to be about, decided against the city facet.
 *
 *   `city`        exactly one city, and the facet offers it.
 *   `unknown`     exactly one city, and the facet does not offer it.
 *   `generic`     no city, `?cities=all`, or more than one city.
 *   `unavailable` one city, and the facet could not be read.
 *
 * `unknown` is its own state rather than a flavour of `generic` because the two
 * differ in what a crawler should do: a city the facet does not offer has no
 * verified rooms, so the page is real but has nothing to index.
 *
 * `unavailable` is a fourth state for the same kind of reason, in the opposite
 * direction: without the facet this page cannot tell an empty city from a busy
 * one, so it declares NOTHING rather than guessing. Reading it as `unknown`
 * would turn a backend blip into a noindex on every city page for a cache
 * window; reading it as `generic` would have a real city page assert that it is
 * a duplicate of the directory root. A missing canonical is the only honest
 * answer, and it is also the recoverable one.
 *
 * An EMPTY facet is unavailable too, not unknown. A facet with no rows cannot
 * tell a city it does not carry from a city it has not loaded, which is the
 * same guard `VenueList` applies before it shows the choose-a-city state.
 */
export type VenuesScopeKind = 'city' | 'unknown' | 'generic' | 'unavailable'

export interface VenuesScope {
  kind: VenuesScopeKind
  /** The facet's spelling of the city, present only for `city`. */
  city: CityState | null
  /**
   * The city's verified-room count, present only for `city`. It bounds the
   * city's page space; see `buildVenuesMetadata`.
   */
  rooms: number
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
  if (selected.length !== 1) return { kind: 'generic', city: null, rooms: 0 }
  if (!facet || facet.length === 0) {
    return { kind: 'unavailable', city: null, rooms: 0 }
  }
  const city = facetCityFor(selected, facet)
  if (!city) return { kind: 'unknown', city: null, rooms: 0 }
  const row = facet.find(c => c.city === city.city && c.state === city.state)
  return { kind: 'city', city, rooms: row?.venue_count ?? 0 }
}

/**
 * The cities the URL names, read the way the list reads them.
 *
 * `?cities=` is the wire format every surface shares; `?city=`/`?state=` are the
 * legacy single-city pair that predates it and that `VenueList` still honours
 * (read-only, and only when `?cities=` is absent). BOTH are read here, because
 * a legacy deep link renders a city heading, so it has to get that city's title
 * and canonical too — and, when the facet does not know the city, the same
 * noindex the modern spelling gets.
 */
export function venuesUrlCities(
  params: Record<string, string | string[] | undefined>
): CityState[] {
  const fromCities = parseCitiesParam(firstParam(params.cities))
  if (fromCities.length > 0 || params.cities !== undefined) return fromCities

  const city = firstParam(params.city)?.trim()
  const state = firstParam(params.state)?.trim()
  return city && state ? [{ city, state }] : []
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
 *
 * THE PAGE IN THE CANONICAL IS BOUNDED BY THE CITY'S OWN PAGE SPACE. `?page=`
 * is free text, so without this every city would offer MAX_ARCHIVE_PAGE
 * distinct URLs that each declare THEMSELVES canonical, nearly all of them
 * empty — a crawler trap this surface did not have while every variant
 * canonicalized to the root. The facet's room count sizes that space without a
 * second request, and a page past the end canonicalizes to the city's first
 * page, which is what the body of such a page tells the reader to go back to.
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

  let canonical: string | undefined
  if (scope.kind === 'city' && scope.city) {
    const lastPage = Math.max(1, Math.ceil(scope.rooms / VENUES_PAGE_SIZE))
    canonical = venuesCityCanonical(
      VENUES_ROOT,
      buildCitiesParam([scope.city]),
      page <= lastPage ? page : 1
    )
  } else if (scope.kind === 'generic') {
    canonical = listRootCanonical(VENUES_ROOT)
  }
  // `unknown` and `unavailable` name no canonical, for two different reasons.
  // An unavailable facet has nothing to name (above). An unknown city is asking
  // NOT to be indexed, and a noindex beside a canonical pointing at a DIFFERENT
  // url invites that noindex to be consolidated onto the target — which here
  // would be the directory root, the one page on this surface that must stay
  // indexed.

  return {
    title,
    description,
    ...(canonical ? { alternates: { canonical } } : {}),
    // A city with no verified rooms is a real page with nothing on it: it stays
    // reachable and its links stay followable, and it asks not to be indexed.
    // Every other state is the directory itself, which is indexable.
    ...(scope.kind === 'unknown'
      ? { robots: { index: false, follow: true } }
      : {}),
    // The same URL as the canonical: a shared link and the indexed page are one
    // address. With no canonical to name there is no address to assert either.
    openGraph: {
      title: `${title} | Psychic Homily`,
      description,
      ...(canonical ? { url: canonical } : {}),
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
