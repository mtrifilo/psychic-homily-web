import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { ReactElement } from 'react'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'
import { findElements } from '@/test/elementTree'
import { JsonLd } from '@/components/seo/JsonLd'
import { SITE_URL } from '@/lib/seo/siteMetadata'

// `notFound()` in Next.js throws to halt rendering; mirrored so a test can tell
// that execution stopped.
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND_SENTINEL)
  }),
}))

// These tests CALL `ReleasePage()` and walk the returned element tree; nothing
// is rendered, so the lazily imported ReleaseDetail never loads.

import ReleasePage, { generateMetadata } from './page'
import { resetServerFetchReportThrottleForTests } from '@/lib/ssr/reportServerFetchFailure'

function buildRelease(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    title: 'Eyes Shut',
    slug: 'eyes-shut',
    release_type: 'ep',
    release_year: 2019,
    release_date: null,
    cover_art_url: 'https://coverartarchive.org/release-group/abc/front.jpg',
    cover_art_source: 'cover_art_archive',
    cover_art_source_url: null,
    description: null,
    artists: [
      { id: 1, slug: 'tim-atlas', name: 'Tim Atlas', role: 'main' },
      { id: 2, slug: 'a-producer', name: 'A Producer', role: 'producer' },
    ],
    labels: [{ id: 3, name: 'Nettwerk', slug: 'nettwerk' }],
    external_links: [
      { id: 1, platform: 'bandcamp', url: 'https://timatlas.bandcamp.com/album/eyes-shut' },
      { id: 2, platform: 'discogs', url: 'https://www.discogs.com/master/1-Eyes-Shut' },
      { id: 3, platform: 'spotify', url: 'https://open.spotify.com/album/abc' },
    ],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const fetchMock = vi.fn()

beforeEach(() => {
  resetServerFetchReportThrottleForTests()
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const metadataFor = (slug: string) => generateMetadata({ params: Promise.resolve({ slug }) })

describe('releases/[slug] generateMetadata', () => {
  it('names the main artist, type, year, label and listen services', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildRelease()))

    const meta = await metadataFor('eyes-shut')

    expect(meta.title).toBe('Eyes Shut by Tim Atlas (2019)')
    expect(meta.description).toBe(
      'Eyes Shut, EP by Tim Atlas (2019). Label: Nettwerk. Listen: Bandcamp, Spotify.'
    )
    expect(meta.openGraph).toMatchObject({
      title: 'Eyes Shut by Tim Atlas (2019)',
      description:
        'Eyes Shut, EP by Tim Atlas (2019). Label: Nettwerk. Listen: Bandcamp, Spotify.',
      type: 'website',
      url: '/releases/eyes-shut',
    })
    expect(meta.alternates?.canonical).toBe('https://psychichomily.com/releases/eyes-shut')
  })

  it('joins two main artists and drops the clauses a sparse release lacks', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildRelease({
          title: 'Split',
          release_type: 'lp',
          release_year: null,
          artists: [
            { id: 1, slug: 'band-a', name: 'Band A', role: 'main' },
            { id: 2, slug: 'guest', name: 'Guest', role: 'featured' },
            { id: 3, slug: 'band-b', name: 'Band B', role: 'main' },
          ],
          labels: null,
          external_links: null,
        })
      )
    )

    const meta = await metadataFor('split')

    expect(meta.title).toBe('Split by Band A and Band B')
    expect(meta.description).toBe('Split, LP by Band A and Band B.')
    expect(meta.openGraph).toMatchObject({
      title: 'Split by Band A and Band B',
      description: 'Split, LP by Band A and Band B.',
    })
  })

  it('names the first of three main artists with "and others"', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildRelease({
          title: 'Comp',
          release_type: 'compilation',
          release_year: 2021,
          artists: ['Band A', 'Band B', 'Band C'].map((name, i) => ({
            id: i,
            slug: name,
            name,
            role: 'main',
          })),
          labels: [],
          external_links: [],
        })
      )
    )

    const meta = await metadataFor('comp')

    expect(meta.title).toBe('Comp by Band A and others (2021)')
    expect(meta.description).toBe('Comp, Compilation by Band A and others (2021).')
  })

  it('drops the type clause, without throwing, for a stored type that is an inherited object key', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildRelease({ release_type: 'constructor' })))

    const meta = await metadataFor('eyes-shut')

    expect(meta.description).toBe(
      'Eyes Shut by Tim Atlas (2019). Label: Nettwerk. Listen: Bandcamp, Spotify.'
    )
  })

  it('falls back to generic metadata when the release is not found', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await metadataFor('missing')

    expect(meta.title).toBe('Release')
    expect(meta.description).toBe('View release details')
  })
})

function albumSchemasIn(page: ReactElement): Array<Record<string, unknown>> {
  return findElements(page, node => node.type === JsonLd)
    .map(node => (node.props as { data: Record<string, unknown> }).data)
    .filter(data => data['@type'] === 'MusicAlbum')
}

describe('releases/[slug] page JSON-LD', () => {
  it('emits one MusicAlbum naming the credited artists with their pages', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildRelease()))

    const page = await ReleasePage({ params: Promise.resolve({ slug: 'eyes-shut' }) })

    expect(albumSchemasIn(page)).toEqual([
      {
        '@context': 'https://schema.org',
        '@type': 'MusicAlbum',
        name: 'Eyes Shut',
        byArtist: [
          { '@type': 'MusicGroup', name: 'Tim Atlas', url: `${SITE_URL}/artists/tim-atlas` },
        ],
        datePublished: '2019',
        albumReleaseType: 'https://schema.org/EPRelease',
        image: 'https://coverartarchive.org/release-group/abc/front.jpg',
        url: `${SITE_URL}/releases/eyes-shut`,
      },
    ])
  })

  it('omits the properties a sparse release has no value for', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildRelease({
          release_type: 'compilation',
          release_year: null,
          cover_art_url: null,
          artists: null,
        })
      )
    )

    const page = await ReleasePage({ params: Promise.resolve({ slug: 'eyes-shut' }) })

    expect(albumSchemasIn(page)).toEqual([
      {
        '@context': 'https://schema.org',
        '@type': 'MusicAlbum',
        name: 'Eyes Shut',
        url: `${SITE_URL}/releases/eyes-shut`,
      },
    ])
  })

  it('halts with notFound() before any JSON-LD when the release is not found', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(
      ReleasePage({ params: Promise.resolve({ slug: 'missing' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)
  })
})
