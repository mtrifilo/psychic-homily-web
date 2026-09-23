import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))


// Stub the heavy venues feature module so invoking generateMetadata doesn't
// pull in the real VenueDetail render path.
vi.mock('@/features/venues', () => ({
  VenueDetail: (): null => null,
}))

import { generateMetadata } from './page'

function buildVenue(overrides: Record<string, unknown> = {}) {
  return {
    id: 77,
    timezone: 'America/Phoenix',
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    address: '2303 E Indian School Rd',
    city: 'Phoenix',
    state: 'AZ',
    zip_code: '85016',
    ...overrides,
  }
}

const fetchMock = vi.fn()
const upcomingFetchMock = vi.fn()

/** The venue shows list the metadata reads its `Next` clause from. */
function upcomingResponse(shows: unknown[]): Response {
  return new Response(
    JSON.stringify({ shows, venue_id: 77, total: shows.length, limit: 7, offset: 0, year: 0 }),
    { status: 200, headers: { 'content-type': 'application/json' } }
  )
}

// 8:00 PM on Friday, September 11, 2026 in Phoenix.
function upcomingShow(overrides: Record<string, unknown> = {}) {
  return {
    id: 901,
    slug: 'next-show',
    title: 'Next Show',
    event_date: '2026-09-12T03:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    is_cancelled: false,
    is_sold_out: false,
    artists: [
      { id: 1, slug: 'opener', name: 'Opener', is_headliner: false },
      { id: 2, slug: 'top-billing', name: 'Top Billing', is_headliner: true },
    ],
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  upcomingFetchMock.mockResolvedValue(upcomingResponse([]))
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    if (String(url).includes('/shows?')) return upcomingFetchMock(url, init)
    return fetchMock(url, init)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('venues/[slug] generateMetadata', () => {
  it('puts the city and state in the title', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.title).toBe('The Rebel Lounge · Phoenix, AZ')
  })

  it('names the next show in the description', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    upcomingFetchMock.mockResolvedValueOnce(upcomingResponse([upcomingShow()]))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toBe(
      'Upcoming shows at The Rebel Lounge in Phoenix, AZ. Next: Top Billing, Sep 11.'
    )
  })

  it('asks for the venue upcoming shows by id', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(upcomingFetchMock).toHaveBeenCalledTimes(1)
    const url = new URL(String(upcomingFetchMock.mock.calls[0][0]))
    expect(url.pathname).toMatch(/\/venues\/77\/shows$/)
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
  })

  it('skips a cancelled show when naming the next one', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    upcomingFetchMock.mockResolvedValueOnce(
      upcomingResponse([
        upcomingShow({
          is_cancelled: true,
          artists: [{ id: 3, slug: 'called-off', name: 'Called Off', is_headliner: true }],
        }),
        upcomingShow(),
      ])
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toContain('Next: Top Billing, Sep 11.')
    expect(meta.description).not.toContain('Called Off')
  })

  it('omits the Next clause when the venue has no upcoming show', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toBe('Upcoming shows at The Rebel Lounge in Phoenix, AZ.')
  })

  it('omits the Next clause when the upcoming read fails', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    upcomingFetchMock.mockResolvedValueOnce(errorResponse(500))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.title).toBe('The Rebel Lounge · Phoenix, AZ')
    expect(meta.description).toBe('Upcoming shows at The Rebel Lounge in Phoenix, AZ.')
  })

  it('makes no upcoming read for a venue without a usable id', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue({ id: 'not-a-number' })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(upcomingFetchMock).not.toHaveBeenCalled()
    expect(meta.description).toBe('Upcoming shows at The Rebel Lounge in Phoenix, AZ.')
  })

  it('sets the canonical URL to https://psychichomily.com/venues/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/venues/the-rebel-lounge'
    )
  })

  it('mirrors the title and description into openGraph and sets url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.openGraph?.title).toBe(meta.title)
    expect(meta.openGraph?.description).toBe(meta.description)
    expect(meta.openGraph?.url).toBe('/venues/the-rebel-lounge')
    expect((meta.openGraph as { type?: string })?.type).toBe('website')
  })

  it('falls back to the "Venue" title when the venue is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Venue')
    expect(meta.description).toBe('View venue details and upcoming shows')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
    expect(meta.openGraph).toBeUndefined()
  })
})
