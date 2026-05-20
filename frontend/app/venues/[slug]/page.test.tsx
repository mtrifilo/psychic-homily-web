import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// Stub the heavy venues feature module so invoking generateMetadata doesn't
// pull in the real VenueDetail render path.
vi.mock('@/features/venues', () => ({
  VenueDetail: () => null,
}))

import { generateMetadata } from './page'

function buildVenue(overrides: Record<string, unknown> = {}) {
  return {
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    address: '2303 E Indian School Rd',
    city: 'Phoenix',
    state: 'AZ',
    zip_code: '85016',
    ...overrides,
  }
}

function okResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as unknown as Response
}

function errorResponse(status: number): Response {
  return { ok: false, status, json: async () => ({}) } as unknown as Response
}

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('venues/[slug] generateMetadata', () => {
  it('uses the venue name as the title when the venue is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.title).toBe('The Rebel Lounge')
  })

  it('builds the description from name, city, and state', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toBe(
      'The Rebel Lounge in Phoenix, AZ - upcoming shows and venue details'
    )
  })

  it('sets the canonical URL to https://psychichomily.com/venues/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/venues/the-rebel-lounge'
    )
  })

  it('sets openGraph title/description/url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.openGraph?.title).toBe('The Rebel Lounge')
    expect(meta.openGraph?.description).toBe('View upcoming shows at The Rebel Lounge')
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
