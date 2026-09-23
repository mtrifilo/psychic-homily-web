import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))


// Stub the heavy artists feature module so invoking generateMetadata doesn't
// pull in the real ArtistDetail render path.
vi.mock('@/features/artists', () => ({
  ArtistDetail: (): null => null,
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

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('artists/[slug] generateMetadata', () => {
  it('puts the location in the title when the artist has one', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.title).toBe('Headliner Band · Phoenix, AZ')
  })

  it('puts the location in the description when the artist has one', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.description).toBe(
      'Headliner Band from Phoenix, AZ: shows, similar artists and connections on Psychic Homily'
    )
  })

  it('uses the bare name and no from clause when the artist has no location', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildArtist({ name: 'Cape Fury', slug: 'cape-fury', city: null, state: null }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'cape-fury' }) })

    expect(meta.title).toBe('Cape Fury')
    expect(meta.description).toBe(
      'Cape Fury: shows, similar artists and connections on Psychic Homily'
    )
  })

  it('makes no request beyond the artist read', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('sets the canonical URL to https://psychichomily.com/artists/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/artists/headliner-band'
    )
  })

  it('mirrors the title and description into openGraph and sets url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.openGraph?.title).toBe(meta.title)
    expect(meta.openGraph?.description).toBe(meta.description)
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
