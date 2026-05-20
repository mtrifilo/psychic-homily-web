import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// Stub the heavy artists feature module so invoking generateMetadata doesn't
// pull in the real ArtistDetail render path.
vi.mock('@/features/artists', () => ({
  ArtistDetail: () => null,
}))

import { generateMetadata } from './page'

function buildArtist(overrides: Record<string, unknown> = {}) {
  return {
    name: 'Headliner Band',
    slug: 'headliner-band',
    city: 'Phoenix',
    state: 'AZ',
    social: {},
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

describe('artists/[slug] generateMetadata', () => {
  it('uses the artist name as the title when the artist is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.title).toBe('Headliner Band')
  })

  it('builds the description from the artist name', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.description).toBe(
      'Headliner Band - upcoming shows and artist details on Psychic Homily'
    )
  })

  it('sets the canonical URL to https://psychichomily.com/artists/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/artists/headliner-band'
    )
  })

  it('sets openGraph title/description/url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.openGraph?.title).toBe('Headliner Band')
    expect(meta.openGraph?.description).toBe('View upcoming shows featuring Headliner Band')
    expect(meta.openGraph?.url).toBe('/artists/headliner-band')
    expect((meta.openGraph as { type?: string })?.type).toBe('website')
  })

  it('falls back to the "Artist" title when the artist is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Artist')
    expect(meta.description).toBe('View artist details and upcoming shows')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
    expect(meta.openGraph).toBeUndefined()
  })
})
