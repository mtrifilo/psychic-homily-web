import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

// generateMetadata calls getCollection, which reads the auth cookie (PSY-551
// cookie-forward). Return an empty cookie store so the anonymous (ISR) fetch
// path is exercised.
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({
    get: vi.fn(() => undefined),
  })),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// Stub the heavy collections feature module so invoking generateMetadata
// doesn't pull in the real CollectionDetail render path.
vi.mock('@/features/collections/components', () => ({
  CollectionDetail: () => null,
}))

import { generateMetadata } from './page'

function buildCollection(overrides: Record<string, unknown> = {}) {
  return {
    title: 'Best Phoenix Shows',
    slug: 'best-phoenix-shows',
    description: 'A hand-picked list of must-see shows.',
    creator_name: 'matt',
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

describe('collections/[slug] generateMetadata', () => {
  it('uses the collection title as the title when the collection is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildCollection()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'best-phoenix-shows' }) })

    expect(meta.title).toBe('Best Phoenix Shows')
  })

  it('uses the description when present', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildCollection()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'best-phoenix-shows' }) })

    expect(meta.description).toBe('A hand-picked list of must-see shows.')
  })

  it('truncates a long description at 160 chars (no ellipsis)', async () => {
    const longDescription = 'z'.repeat(300)
    fetchMock.mockResolvedValueOnce(okResponse(buildCollection({ description: longDescription })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'best-phoenix-shows' }) })

    expect(meta.description).toBe('z'.repeat(160))
    expect(meta.description).toHaveLength(160)
  })

  it('generates a description from the title when none is provided', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildCollection({ description: undefined })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'best-phoenix-shows' }) })

    expect(meta.description).toBe('Best Phoenix Shows - a curated collection on Psychic Homily')
  })

  it('sets the canonical URL to https://psychichomily.com/collections/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildCollection()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'best-phoenix-shows' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/collections/best-phoenix-shows'
    )
  })

  it('sets openGraph title/description/url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildCollection()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'best-phoenix-shows' }) })

    expect(meta.openGraph?.title).toBe('Best Phoenix Shows')
    expect(meta.openGraph?.description).toBe('A hand-picked list of must-see shows.')
    expect(meta.openGraph?.url).toBe('/collections/best-phoenix-shows')
    expect((meta.openGraph as { type?: string })?.type).toBe('website')
  })

  it('falls back to the "Collection" title when the collection is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Collection')
    expect(meta.description).toBe('View collection details')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
    expect(meta.openGraph).toBeUndefined()
  })
})
