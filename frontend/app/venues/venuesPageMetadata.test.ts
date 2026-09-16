import { describe, expect, it } from 'vitest'
import {
  buildVenuesMetadata,
  firstParam,
  resolveVenuesPage,
  resolveVenuesScope,
  venuesUrlCities,
  VENUES_GENERIC_TITLE,
  type FacetCity,
} from './venuesPageMetadata'
import { VENUES_PAGE_SIZE } from '@/features/venues/venuesListNavigation'

/**
 * The facet the directory resolves a URL's city against. Phoenix is sized to
 * two pages and Tucson to one, so the page-space bound below is exercised
 * against a real boundary rather than against zero.
 */
const FACET: FacetCity[] = [
  { city: 'Phoenix', state: 'AZ', venue_count: VENUES_PAGE_SIZE + 1 },
  { city: 'Tucson', state: 'AZ', venue_count: 3 },
  { city: 'New York', state: 'NY', venue_count: 12 },
]

/** The whole URL, read the way `generateMetadata` reads it. */
type Params = Record<string, string | string[] | undefined>

function seoOf(params: Params, facet: FacetCity[] | null = FACET) {
  const metadata = buildVenuesMetadata(
    resolveVenuesScope(venuesUrlCities(params), facet),
    resolveVenuesPage(params)
  )
  return {
    title: metadata.title,
    description: metadata.description,
    canonical: metadata.alternates?.canonical,
    robots: metadata.robots,
  }
}

const scopeOf = (params: Params, facet: FacetCity[] | null = FACET) =>
  resolveVenuesScope(venuesUrlCities(params), facet)

describe('venuesUrlCities', () => {
  it('reads the shared ?cities= wire format', () => {
    expect(venuesUrlCities({ cities: 'Phoenix,AZ' })).toEqual([
      { city: 'Phoenix', state: 'AZ' },
    ])
  })

  // The legacy pair renders a city heading, so it has to reach the title and
  // the canonical too.
  it('reads the legacy ?city=/?state= pair', () => {
    expect(venuesUrlCities({ city: 'Phoenix', state: 'AZ' })).toEqual([
      { city: 'Phoenix', state: 'AZ' },
    ])
  })

  // Same precedence the list applies: a present ?cities= wins outright, even
  // when it parses to nothing.
  it('lets a present ?cities= shut out the legacy pair', () => {
    expect(
      venuesUrlCities({ cities: 'all', city: 'Phoenix', state: 'AZ' })
    ).toEqual([])
  })

  it('needs both halves of the legacy pair', () => {
    expect(venuesUrlCities({ city: 'Phoenix' })).toEqual([])
    expect(venuesUrlCities({ state: 'AZ' })).toEqual([])
  })
})

describe('resolveVenuesScope', () => {
  it('takes the facet spelling, not the URL casing', () => {
    expect(scopeOf({ cities: 'phoenix,az' })).toEqual({
      kind: 'city',
      city: { city: 'Phoenix', state: 'AZ' },
      rooms: VENUES_PAGE_SIZE + 1,
    })
  })

  it('reads a city the facet does not offer as unknown', () => {
    expect(scopeOf({ cities: 'Flagstaff,AZ' })).toEqual({
      kind: 'unknown',
      city: null,
      rooms: 0,
    })
  })

  it.each([
    ['absent', {}],
    ['all', { cities: 'all' }],
    ['multi-city', { cities: 'Phoenix,AZ|Tucson,AZ' }],
    ['malformed', { cities: 'Phoenix' }],
  ])('is generic when the URL names no single city (%s)', (_label, params) => {
    expect(scopeOf(params)).toEqual({ kind: 'generic', city: null, rooms: 0 })
  })

  // Reading a missing facet as `unknown` would turn one backend blip into a
  // noindex on every city page for a whole cache window.
  it.each([
    ['unreadable', null],
    ['empty', [] as FacetCity[]],
  ])('is unavailable, not unknown, when the facet is %s', (_label, facet) => {
    expect(scopeOf({ cities: 'Phoenix,AZ' }, facet)).toEqual({
      kind: 'unavailable',
      city: null,
      rooms: 0,
    })
  })
})

describe('resolveVenuesPage', () => {
  it('is page one when the parameter is absent or not a number', () => {
    expect(resolveVenuesPage({})).toBe(1)
    expect(resolveVenuesPage({ page: 'nonsense' })).toBe(1)
  })

  it('reads a page the reader can be on', () => {
    expect(resolveVenuesPage({ page: '3' })).toBe(3)
  })

  it('clamps a page below one', () => {
    expect(resolveVenuesPage({ page: '-4' })).toBe(1)
  })

  // The browser reads `?page=` with nuqs, whose parseInt carries no radix. A
  // hand-rolled `parseInt(v, 10)` here would name page 1 while the list renders
  // page 16, and the canonical would disagree with the rows under it.
  it('reads a hex-looking page the way the browser does', () => {
    expect(resolveVenuesPage({ page: '0x10' })).toBe(16)
  })
})

describe('the directory metadata', () => {
  it('names one city in the title, the description and a self canonical', () => {
    expect(seoOf({ cities: 'Phoenix,AZ' })).toEqual({
      title: 'Venues in Phoenix, AZ',
      description:
        'Live-music rooms in Phoenix, AZ: upcoming shows, quiet rooms, links.',
      canonical: 'https://psychichomily.com/venues?cities=Phoenix%2CAZ',
      robots: undefined,
    })
  })

  it('names a legacy deep link the same way', () => {
    expect(seoOf({ city: 'phoenix', state: 'az' }).title).toBe(
      'Venues in Phoenix, AZ'
    )
  })

  // The owner-locked directory exception to the site's canonicalize-to-root
  // pagination policy: each page of a city is its own document.
  it('carries the page into the canonical from page two', () => {
    expect(seoOf({ cities: 'Phoenix,AZ', page: '2' }).canonical).toBe(
      'https://psychichomily.com/venues?cities=Phoenix%2CAZ&page=2'
    )
  })

  it('leaves page one out of the canonical', () => {
    expect(seoOf({ cities: 'Phoenix,AZ', page: '1' }).canonical).toBe(
      'https://psychichomily.com/venues?cities=Phoenix%2CAZ'
    )
  })

  // Without this bound every city offers MAX_ARCHIVE_PAGE distinct URLs that
  // each declare THEMSELVES canonical, nearly all of them empty.
  it('canonicalizes a page past the city to the city itself', () => {
    // Tucson holds three rooms, so it has exactly one page.
    expect(seoOf({ cities: 'Tucson,AZ', page: '7' }).canonical).toBe(
      'https://psychichomily.com/venues?cities=Tucson%2CAZ'
    )
    expect(seoOf({ cities: 'Phoenix,AZ', page: '873' }).canonical).toBe(
      'https://psychichomily.com/venues?cities=Phoenix%2CAZ'
    )
  })

  // Order and filter states reorder or narrow ONE city's rooms rather than
  // answering a different question, so they never reach the canonical.
  it('drops sort and tag state from a city canonical', () => {
    const canonical = seoOf({
      cities: 'Phoenix,AZ',
      page: '2',
      sort: 'name',
      tags: 'punk',
      tag_match: 'any',
    }).canonical
    expect(canonical).toBe(
      'https://psychichomily.com/venues?cities=Phoenix%2CAZ&page=2'
    )
  })

  it('spells a space the way the page links it', () => {
    expect(seoOf({ cities: 'New York,NY' }).canonical).toBe(
      'https://psychichomily.com/venues?cities=New+York%2CNY'
    )
  })

  // The share card and the indexed page are one address.
  it('points Open Graph at the canonical', () => {
    const city = buildVenuesMetadata(scopeOf({ cities: 'Phoenix,AZ' }), 2)
    expect(city.openGraph?.url).toBe(city.alternates?.canonical)
    expect(buildVenuesMetadata(scopeOf({}), 1).openGraph?.url).toBe(
      'https://psychichomily.com/venues'
    )
  })

  // With no canonical to name there is no address to assert on the card either.
  it('names no Open Graph url where it names no canonical', () => {
    for (const params of [
      { cities: 'Flagstaff,AZ' },
      { cities: 'Phoenix,AZ' },
    ]) {
      const facet = params.cities === 'Phoenix,AZ' ? null : FACET
      const metadata = buildVenuesMetadata(
        scopeOf(params, facet),
        resolveVenuesPage(params)
      )
      expect(metadata.alternates?.canonical).toBeUndefined()
      expect(metadata.openGraph?.url).toBeUndefined()
    }
  })

  it.each([
    ['bare', {}],
    ['all', { cities: 'all' }],
    ['multi-city', { cities: 'Phoenix,AZ|Tucson,AZ' }],
  ])('keeps the generic title and root canonical (%s)', (_label, params) => {
    expect(seoOf(params)).toEqual({
      title: VENUES_GENERIC_TITLE,
      description: 'Browse music venues and discover upcoming shows.',
      canonical: 'https://psychichomily.com/venues',
      robots: undefined,
    })
  })

  // A city with no verified rooms is a real page with nothing on it: reachable,
  // its links followable, and not worth an index entry. It names NO canonical:
  // a noindex beside a canonical pointing elsewhere invites the noindex to be
  // consolidated onto the target, which here is the directory root.
  it('asks crawlers to skip a city the facet does not offer', () => {
    expect(seoOf({ cities: 'Flagstaff,AZ' })).toEqual({
      title: VENUES_GENERIC_TITLE,
      description: 'Browse music venues and discover upcoming shows.',
      canonical: undefined,
      robots: { index: false, follow: true },
    })
  })

  it('treats a legacy unknown city the same way', () => {
    expect(seoOf({ city: 'Flagstaff', state: 'AZ' }).robots).toEqual({
      index: false,
      follow: true,
    })
  })

  // Without the facet this page cannot tell an empty city from a busy one, so
  // it asserts neither an index posture nor an address.
  it('declares nothing while the facet is unavailable', () => {
    expect(seoOf({ cities: 'Phoenix,AZ' }, null)).toEqual({
      title: VENUES_GENERIC_TITLE,
      description: 'Browse music venues and discover upcoming shows.',
      canonical: undefined,
      robots: undefined,
    })
  })
})

describe('firstParam', () => {
  // First rather than "invalid", because `useSearchParams().get` takes the
  // first too, so a repeated parameter names one page on both sides.
  it('takes the first value of a repeated parameter', () => {
    expect(firstParam(['Phoenix,AZ', 'Tucson,AZ'])).toBe('Phoenix,AZ')
    expect(firstParam('Phoenix,AZ')).toBe('Phoenix,AZ')
    expect(firstParam(undefined)).toBeUndefined()
  })
})
