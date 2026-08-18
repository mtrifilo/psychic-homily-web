import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

// The archive body renders the real client archive; these tests only exercise
// `generateMetadata`, so the render path is stubbed out the way the sibling
// venue page test does.
vi.mock('@/features/venues/components/VenuePastShows', () => ({
  VenuePastShows: (): null => null,
  VENUE_PAST_SHOWS_ANCHOR: 'venue-past-shows',
}))

import VenueYearArchivePage, { generateMetadata } from './page'

function buildVenue(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    city: 'Phoenix',
    state: 'AZ',
    ...overrides,
  }
}

function buildYears(years: Array<{ year: number; count: number }>) {
  return { venue_id: 7, time_filter: 'past', years }
}

const fetchMock = vi.fn()

/** The archive reads the venue first, then the past-year histogram. */
function mockVenueAndYears(
  venue: unknown,
  years: Array<{ year: number; count: number }>
) {
  fetchMock.mockResolvedValueOnce(okResponse(venue))
  fetchMock.mockResolvedValueOnce(okResponse(buildYears(years)))
}

const params = (slug: string, year: string) => Promise.resolve({ slug, year })

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('venues/[slug]/shows/[year] generateMetadata', () => {
  it('titles the page with the venue and the year', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 14 }])

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '2025') })

    expect(meta.title).toBe('The Rebel Lounge shows in 2025')
  })

  it('canonicalizes to itself — the year root, never page 1 of anything else', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 14 }])

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '2025') })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/venues/the-rebel-lounge/shows/2025'
    )
  })

  /**
   * The whole point of keeping `?page=` out of the path: intra-year pages
   * canonicalize to the year root by construction.
   *
   * Asserted BEHAVIOURALLY. An earlier version of this test checked
   * `generateMetadata.length === 1`, which is worthless — a destructured
   * parameter counts as one whether or not it destructures `searchParams`, so
   * the exact regression it named would have shipped green. Handing the
   * function a searchParams-carrying argument and demanding the canonical not
   * move is what actually holds the line.
   */
  it('ignores searchParams, so every ?page= of the year shares one canonical', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 161 }])

    const meta = await generateMetadata({
      params: params('the-rebel-lounge', '2025'),
      searchParams: Promise.resolve({ page: '4' }),
    } as unknown as Parameters<typeof generateMetadata>[0])

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/venues/the-rebel-lounge/shows/2025'
    )
  })

  it('describes the archive with the venue, the place and the count', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 14 }])

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '2025') })

    expect(meta.description).toBe(
      'Every show we have on record at The Rebel Lounge in Phoenix, AZ during 2025 — 14 shows.'
    )
  })

  it('singularizes a one-show year', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2019, count: 1 }])

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '2019') })

    expect(meta.description).toContain('— 1 show.')
  })

  it('points openGraph at the same canonical URL', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 14 }])

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '2025') })

    expect(meta.openGraph?.url).toBe(
      'https://psychichomily.com/venues/the-rebel-lounge/shows/2025'
    )
    expect(meta.openGraph?.title).toBe('The Rebel Lounge shows in 2025')
  })

  it('noindexes a year the venue has no past shows in', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 14 }])

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '1999') })

    expect(meta.title).toBe('Shows not found')
    expect(meta.robots).toEqual({ index: false, follow: false })
  })

  it('noindexes a venue the backend says is gone', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: params('no-such-venue', '2025') })

    expect(meta.robots).toEqual({ index: false, follow: false })
    // Both reads go out together — the histogram does not wait on the venue.
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  /**
   * The sharp edge of this route: the proxy fails OPEN on a 5xx, so a backend
   * blip lands real archive URLs on this page. If the head answered that with
   * `noindex` it would be issuing a de-index instruction — at HTTP 200, with no
   * retry signal — for a URL that was fine a moment ago. "Could not tell" must
   * never be published as "not there".
   */
  it.each([500, 429, 503])(
    'does NOT noindex when the backend answered %d',
    async status => {
      fetchMock.mockResolvedValueOnce(errorResponse(status))
      fetchMock.mockResolvedValueOnce(errorResponse(status))

      const meta = await generateMetadata({
        params: params('the-rebel-lounge', '2025'),
      })

      expect(meta.robots).toBeUndefined()
      expect(meta.alternates?.canonical).toBe(
        'https://psychichomily.com/venues/the-rebel-lounge/shows/2025'
      )
    }
  )

  it('does NOT noindex when only the histogram is unavailable', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    fetchMock.mockResolvedValueOnce(errorResponse(500))

    const meta = await generateMetadata({ params: params('the-rebel-lounge', '2025') })

    expect(meta.robots).toBeUndefined()
    expect(meta.title).toBe('The Rebel Lounge shows in 2025')
    // No count is claimed when nothing could be counted.
    expect(meta.description).toBe(
      'Every show we have on record at The Rebel Lounge in Phoenix, AZ during 2025.'
    )
  })

  /**
   * A path segment is not a form field. Anything that is not exactly four
   * digits has to be a dead end, or `/shows/%202025%20` becomes a second
   * address for the same rows — and it must not cost a backend round trip to
   * say so.
   */
  it.each(['2025abc', ' 2025 ', '25', '20255', '', 'shows'])(
    'rejects the year segment %p without fetching',
    async segment => {
      const meta = await generateMetadata({
        params: params('the-rebel-lounge', segment),
      })

      expect(meta.title).toBe('Shows not found')
      expect(meta.robots).toEqual({ index: false, follow: false })
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  /** The upper bound the backend enforces on `year`; above it is a 422. */
  it('rejects a four-digit year outside the archive range', async () => {
    const meta = await generateMetadata({ params: params('the-rebel-lounge', '0000') })

    expect(meta.title).toBe('Shows not found')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

/**
 * What the page BODY reads, which since PSY-1770 is a shorter list than it was.
 *
 * The rows moved under a Suspense boundary in `VenueYearArchiveContent` because
 * they depend on `?page=`, and reading search params in this body would make the
 * whole route dynamic and cost it the prerendered shell PSY-1753/1756 measured.
 * These assertions hold that line: two reads here, whatever the URL asks for,
 * and no dependence on the search params at all.
 */
describe('venues/[slug]/shows/[year] page body', () => {
  const noSearchParams = Promise.resolve({})

  it('reads the venue and the histogram, and nothing else', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 161 }])

    await VenueYearArchivePage({
      params: params('the-rebel-lounge', '2025'),
      searchParams: noSearchParams,
    })

    expect(fetchMock).toHaveBeenCalledTimes(2)
    const urls = fetchMock.mock.calls.map(call => String(call[0]))
    expect(urls.some(url => url.includes('/shows/years'))).toBe(true)
    // The rows are the read that moved. A `limit=` on this list is the
    // signature of the page-1 fetch.
    expect(urls.some(url => url.includes('limit='))).toBe(false)
  })

  /**
   * It does not merely ignore the search params — it never awaits them. A body
   * that did would take the route dynamic even while reaching the same answer,
   * which is exactly the regression the boundary exists to prevent and the one a
   * behavioural assertion can catch without a build.
   */
  it('does not await searchParams', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 161 }])
    const searchParams = {
      then: vi.fn(),
    } as unknown as Promise<Record<string, string | string[] | undefined>>

    await VenueYearArchivePage({
      params: params('the-rebel-lounge', '2025'),
      searchParams,
    })

    expect(searchParams.then).not.toHaveBeenCalled()
  })

  it('makes the same two reads on a deep page', async () => {
    mockVenueAndYears(buildVenue(), [{ year: 2025, count: 161 }])

    await VenueYearArchivePage({
      params: params('the-rebel-lounge', '2025'),
      searchParams: Promise.resolve({ page: '4' }),
    })

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
