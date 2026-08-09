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

import { generateMetadata } from './page'

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
   * The whole point of keeping `?page=` out of the path: this function takes
   * only params, so there is no code path on which a page number can reach the
   * canonical. Intra-year pages therefore canonicalize to the year root by
   * construction rather than by remembering to strip something.
   */
  it('takes no searchParams, so every ?page= of the year shares this canonical', async () => {
    expect(generateMetadata.length).toBe(1)
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

  it('noindexes a missing venue without asking the backend for a year', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: params('no-such-venue', '2025') })

    expect(meta.robots).toEqual({ index: false, follow: false })
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
