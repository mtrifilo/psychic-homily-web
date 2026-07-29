import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException } = vi.hoisted(() => ({
  captureException: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
}))

// Blog and DJ sets read local MDX off disk; stubbed out so these tests speak
// only to the API-driven families.
vi.mock('@/features/blog', () => ({
  getBlogSlugs: () => [],
  getBlogPost: () => undefined,
  getMixSlugs: () => [],
  getMix: () => undefined,
}))

import sitemap from './sitemap'

const ISO = '2026-07-20T12:00:00Z'

function respondWith(body: unknown, status = 200) {
  return vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  )
}

function urlsOf(entries: Awaited<ReturnType<typeof sitemap>>) {
  return entries.map(e => e.url)
}

describe('sitemap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('maps every entity family onto its URL prefix', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith({
        shows: [{ slug: 'a-show', updated_at: ISO }],
        artists: [{ slug: 'an-artist', updated_at: ISO }],
        venues: [{ slug: 'a-venue', updated_at: ISO }],
      })
    )

    const urls = urlsOf(await sitemap())

    expect(urls).toContain('https://psychichomily.com/shows/a-show')
    expect(urls).toContain('https://psychichomily.com/artists/an-artist')
    expect(urls).toContain('https://psychichomily.com/venues/a-venue')
  })

  it('carries updated_at through to lastModified', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith({
        shows: [{ slug: 'a-show', updated_at: ISO }],
        artists: [],
        venues: [],
      })
    )

    const entry = (await sitemap()).find(
      e => e.url === 'https://psychichomily.com/shows/a-show'
    )

    expect(entry?.lastModified).toEqual(new Date(ISO))
  })

  // The old generator stamped every static entry with `new Date()` on each
  // render, telling crawlers the whole site changed every time. That devalues
  // <lastmod> for the entries that genuinely did change.
  it('does not stamp static pages with a render-time lastModified', async () => {
    vi.stubGlobal('fetch', respondWith({ shows: [], artists: [], venues: [] }))

    const home = (await sitemap()).find(
      e => e.url === 'https://psychichomily.com'
    )

    expect(home).toBeDefined()
    expect(home?.lastModified).toBeUndefined()
  })

  it('keeps an entry but omits lastModified when updated_at is unparseable', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith({
        shows: [{ slug: 'a-show', updated_at: 'not-a-date' }],
        artists: [],
        venues: [],
      })
    )

    const entry = (await sitemap()).find(
      e => e.url === 'https://psychichomily.com/shows/a-show'
    )

    expect(entry).toBeDefined()
    expect(entry?.lastModified).toBeUndefined()
  })

  it('skips entries with no slug — they have no canonical URL', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith({
        shows: [{ slug: '', updated_at: ISO }, { slug: 'real', updated_at: ISO }],
        artists: [],
        venues: [],
      })
    )

    const showUrls = urlsOf(await sitemap()).filter(u =>
      u.startsWith('https://psychichomily.com/shows/')
    )

    expect(showUrls).toEqual(['https://psychichomily.com/shows/real'])
  })

  it('tolerates a null family without throwing', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith({ shows: null, artists: null, venues: null })
    )

    await expect(sitemap()).resolves.toBeInstanceOf(Array)
  })

  // The regression guard. Before this change a failed fetch was caught and
  // turned into `[]`, so the route rendered successfully with an entire entity
  // family missing and no failure signal anywhere (see the backend's
  // contracts.SitemapEntry). Failing the render leaves the last good sitemap up.
  describe('fails closed', () => {
    it('throws when the feed errors rather than emitting a partial sitemap', async () => {
      vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')))

      await expect(sitemap()).rejects.toThrow('network down')
      expect(captureException).toHaveBeenCalled()
    })

    it('throws on a non-ok response', async () => {
      vi.stubGlobal('fetch', respondWith({}, 500))

      await expect(sitemap()).rejects.toThrow(/500/)
      expect(captureException).toHaveBeenCalled()
    })
  })

  it('requests the projection feed, not a heavyweight list endpoint', async () => {
    const fetchMock = respondWith({ shows: [], artists: [], venues: [] })
    vi.stubGlobal('fetch', fetchMock)

    await sitemap()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/sitemap\/entries$/),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })
})
