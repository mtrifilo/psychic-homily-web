import { describe, expect, it } from 'vitest'
import {
  buildVenuesMetadata,
  firstParam,
  parseVenuesCities,
  resolveVenuesPage,
  resolveVenuesScope,
  VENUES_GENERIC_TITLE,
  type FacetCity,
} from './venuesPageMetadata'

/** The facet the directory resolves a URL's city against. */
const FACET: FacetCity[] = [
  { city: 'Phoenix', state: 'AZ' },
  { city: 'Tucson', state: 'AZ' },
  { city: 'New York', state: 'NY' },
]

/** The three facts a state decides together, flattened for one assertion. */
function seoOf(citiesParam?: string, pageParam?: string) {
  const metadata = buildVenuesMetadata(
    resolveVenuesScope(parseVenuesCities(citiesParam), FACET),
    resolveVenuesPage({ cities: citiesParam, page: pageParam })
  )
  return {
    title: metadata.title,
    description: metadata.description,
    canonical: metadata.alternates?.canonical,
    robots: metadata.robots,
  }
}

const scopeOf = (citiesParam?: string, facet: FacetCity[] | null = FACET) =>
  resolveVenuesScope(parseVenuesCities(citiesParam), facet)

describe('resolveVenuesScope', () => {
  it('takes the facet spelling, not the URL casing', () => {
    expect(scopeOf('phoenix,az')).toEqual({
      kind: 'city',
      city: { city: 'Phoenix', state: 'AZ' },
    })
  })

  it('reads a city the facet does not offer as unknown', () => {
    expect(scopeOf('Flagstaff,AZ')).toEqual({ kind: 'unknown', city: null })
  })

  it.each([
    ['absent', undefined],
    ['all', 'all'],
    ['multi-city', 'Phoenix,AZ|Tucson,AZ'],
    ['malformed', 'Phoenix'],
  ])('is generic when the URL names no single city (%s)', (_label, param) => {
    expect(scopeOf(param)).toEqual({ kind: 'generic', city: null })
  })

  // Failing the other way would noindex every city page for a whole cache
  // window on one backend blip.
  it('is generic rather than unknown when the facet is unavailable', () => {
    expect(scopeOf('Phoenix,AZ', null)).toEqual({ kind: 'generic', city: null })
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

  // Clamped the way the list clamps it, so the canonical names the page the
  // reader is actually looking at.
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
    expect(seoOf('Phoenix,AZ')).toEqual({
      title: 'Venues in Phoenix, AZ',
      description:
        'Live-music rooms in Phoenix, AZ: upcoming shows, quiet rooms, links.',
      canonical: 'https://psychichomily.com/venues?cities=Phoenix%2CAZ',
      robots: undefined,
    })
  })

  // The owner-locked directory exception to the site's canonicalize-to-root
  // pagination policy: each page of a city is its own document.
  it('carries the page into the canonical from page two', () => {
    expect(seoOf('Phoenix,AZ', '2').canonical).toBe(
      'https://psychichomily.com/venues?cities=Phoenix%2CAZ&page=2'
    )
  })

  it('leaves page one out of the canonical', () => {
    expect(seoOf('Phoenix,AZ', '1').canonical).toBe(
      'https://psychichomily.com/venues?cities=Phoenix%2CAZ'
    )
  })

  // Order and filter states reorder or narrow ONE city's rooms rather than
  // answering a different question, so they never reach the canonical. There is
  // nothing to drop here because the canonical is BUILT rather than copied.
  it('drops sort and tag state from a city canonical', () => {
    const canonical = seoOf('Phoenix,AZ', '2').canonical
    expect(canonical).not.toContain('sort')
    expect(canonical).not.toContain('tags')
  })

  it('spells a space the way the page links it', () => {
    expect(seoOf('New York,NY').canonical).toBe(
      'https://psychichomily.com/venues?cities=New+York%2CNY'
    )
  })

  // The share card and the indexed page are one address.
  it('points Open Graph at the canonical', () => {
    const metadata = buildVenuesMetadata(scopeOf('Phoenix,AZ'), 2)
    expect(metadata.openGraph?.url).toBe(metadata.alternates?.canonical)
    expect(buildVenuesMetadata(scopeOf(), 1).openGraph?.url).toBe(
      'https://psychichomily.com/venues'
    )
  })

  it.each([
    ['bare', undefined],
    ['all', 'all'],
    ['multi-city', 'Phoenix,AZ|Tucson,AZ'],
  ])('keeps the generic title and root canonical (%s)', (_label, param) => {
    expect(seoOf(param)).toEqual({
      title: VENUES_GENERIC_TITLE,
      description: 'Browse music venues and discover upcoming shows.',
      canonical: 'https://psychichomily.com/venues',
      robots: undefined,
    })
  })

  // A city with no verified rooms is a real page with nothing on it: reachable,
  // its links followable, and not worth an index entry.
  it('asks crawlers to skip a city the facet does not offer', () => {
    expect(seoOf('Flagstaff,AZ')).toEqual({
      title: VENUES_GENERIC_TITLE,
      description: 'Browse music venues and discover upcoming shows.',
      canonical: 'https://psychichomily.com/venues',
      robots: { index: false, follow: true },
    })
  })

  // A paged unknown city is the same nothing: the page number must not talk the
  // canonical off the root.
  it('keeps the root canonical for a paged unknown city', () => {
    expect(seoOf('Flagstaff,AZ', '3').canonical).toBe(
      'https://psychichomily.com/venues'
    )
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
