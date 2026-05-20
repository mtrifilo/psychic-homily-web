import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'

// `notFound()` in Next.js throws a control-flow error to halt rendering. We
// mirror that here so tests can assert BOTH that it was called and that
// execution stopped (no JSON-LD / ShowDetail render after a 404).
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
const notFoundMock = vi.fn(() => {
  throw new Error(NOT_FOUND_SENTINEL)
})

vi.mock('next/navigation', () => ({
  notFound: () => notFoundMock(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// Stub the heavy shows feature module so invoking the page body doesn't pull
// in the real ShowDetail render path — this ticket exercises the page-level
// metadata + JSON-LD + notFound wiring, not ShowDetail.
vi.mock('@/features/shows', () => ({
  ShowDetail: () => null,
}))

import ShowPage, { generateMetadata } from './page'

// Minimal show payload — only the fields generateMetadata / the page body read.
function buildShow(overrides: Record<string, unknown> = {}) {
  return {
    title: 'Test Show',
    event_date: '2026-03-15T20:00:00Z',
    slug: 'test-show',
    description: null,
    is_sold_out: false,
    is_cancelled: false,
    venues: [{ name: 'The Rebel Lounge', slug: 'the-rebel-lounge', city: 'Phoenix', state: 'AZ' }],
    artists: [
      { name: 'Headliner Band', slug: 'headliner-band', is_headliner: true, socials: {} },
    ],
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

describe('generateMetadata', () => {
  it('uses "{headliner} at {venue}" as the title when the show is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.title).toBe('Headliner Band at The Rebel Lounge')
  })

  it('prefers the headliner over the first artist for the title', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildShow({
          artists: [
            { name: 'Opener', slug: 'opener', is_headliner: false, socials: {} },
            { name: 'Real Headliner', slug: 'real-headliner', is_headliner: true, socials: {} },
          ],
        })
      )
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.title).toBe('Real Headliner at The Rebel Lounge')
  })

  it('falls back to the "Show" title when the show is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Show')
    expect(meta.description).toBe('View show details')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
  })

  it('truncates a long description at 155 chars and appends "..."', async () => {
    const longDescription = 'x'.repeat(300)
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ description: longDescription })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.description).toBe('x'.repeat(155) + '...')
    expect(meta.description).toHaveLength(158)
  })

  it('does not append "..." when the description is exactly 155 chars', async () => {
    const exactDescription = 'y'.repeat(155)
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ description: exactDescription })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.description).toBe(exactDescription)
    expect(meta.description).toHaveLength(155)
  })

  it('generates a description from show details when none is provided', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ description: null })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    // Generated form: "{headliner} live at {venue} on {date}".
    expect(meta.description).toContain('Headliner Band live at The Rebel Lounge on')
  })

  it('sets the canonical URL to https://psychichomily.com/shows/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.alternates?.canonical).toBe('https://psychichomily.com/shows/test-show')
    expect(meta.openGraph?.url).toBe('/shows/test-show')
  })
})

describe('ShowPage', () => {
  it('calls notFound() when the slug is empty (post-PSY-733)', async () => {
    await expect(
      ShowPage({ params: Promise.resolve({ slug: '' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(notFoundMock).toHaveBeenCalledTimes(1)
    // Empty slug short-circuits before any fetch.
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('calls notFound() when getShow returns null (404 from API)', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'missing' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(notFoundMock).toHaveBeenCalledTimes(1)
  })

  it('renders without calling notFound() when the show is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(notFoundMock).not.toHaveBeenCalled()
    expect(result).toBeTruthy()
  })
})

describe('getShow error reporting (via the page body)', () => {
  it('reports a 5xx response with Sentry.captureMessage and then 404s', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(503))

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'boom' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'Show page: API returned 503',
      expect.objectContaining({
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug: 'boom', status: 503 },
      })
    )
    // 5xx is reported, not swallowed as an exception.
    expect(Sentry.captureException).not.toHaveBeenCalled()
  })

  it('does NOT report a 404 response to Sentry', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'missing' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureMessage).not.toHaveBeenCalled()
    expect(Sentry.captureException).not.toHaveBeenCalled()
  })

  it('reports a thrown fetch error with Sentry.captureException and then 404s', async () => {
    const networkError = new Error('network down')
    fetchMock.mockRejectedValueOnce(networkError)

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'flaky' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureException).toHaveBeenCalledTimes(1)
    expect(Sentry.captureException).toHaveBeenCalledWith(
      networkError,
      expect.objectContaining({
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug: 'flaky' },
      })
    )
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it('captures a 5xx in generateMetadata as well (both call sites share getShow)', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(500))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'boom' }) })

    // generateMetadata swallows the null and returns the fallback shape.
    expect(meta.title).toBe('Show')
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'Show page: API returned 500',
      expect.objectContaining({ extra: { slug: 'boom', status: 500 } })
    )
  })
})
