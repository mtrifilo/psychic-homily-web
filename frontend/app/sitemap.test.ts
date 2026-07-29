import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException } = vi.hoisted(() => ({
  captureException: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
}))

// Blog and DJ sets read local MDX off disk. Stubbed with one dated and one
// undated post so the `lastModified` fallback on that path is actually
// exercised — this diff changed a missing date from `new Date()` to undefined.
vi.mock('@/features/blog', () => ({
  getBlogSlugs: () => ['dated-post', 'undated-post'],
  getBlogPost: (slug: string) =>
    slug === 'dated-post'
      ? { frontmatter: { date: '2026-03-04T00:00:00Z' } }
      : { frontmatter: {} },
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

  it('carries blog frontmatter dates through, and omits lastModified when absent', async () => {
    vi.stubGlobal('fetch', respondWith({ shows: [], artists: [], venues: [] }))

    const entries = await sitemap()
    const dated = entries.find(e => e.url.endsWith('/blog/dated-post'))
    const undated = entries.find(e => e.url.endsWith('/blog/undated-post'))

    expect(dated?.lastModified).toEqual(new Date('2026-03-04T00:00:00Z'))
    expect(undated).toBeDefined()
    expect(undated?.lastModified).toBeUndefined()
  })

  // The regression guard. Before this change a failed fetch was caught and
  // turned into `[]`, so the route rendered successfully with an entire entity
  // family missing and no failure signal anywhere (see the backend's
  // contracts.SitemapEntry).
  //
  // What failing closed buys depends on how the deploy was built — see the
  // module header in sitemap.ts for the measurements. Built against a healthy
  // backend the route is prerendered, so an outage is survived by serving the
  // last good document. Built against an unreachable backend there is no
  // prerendered body and a request during an outage returns 500.
  //
  // What this guard protects is the same either way, and is the point: a
  // sitemap missing an entity family is never published.
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

    // A 200 whose family is null is the original failure wearing a success
    // code: a document that renders fine and is missing thousands of URLs.
    // Coercing it to [] is what made the first incident invisible, so the
    // shape is rejected rather than tolerated. The generated contract types
    // these families as nullable, so this is reachable, not hypothetical.
    it.each([
      ['null', { shows: null, artists: [], venues: [] }],
      ['absent', { artists: [], venues: [] }],
      ['not an array', { shows: {}, artists: [], venues: [] }],
      ['an empty body', {}],
    ])('throws when a family is %s', async (_label, body) => {
      vi.stubGlobal('fetch', respondWith(body))

      await expect(sitemap()).rejects.toThrow(/missing the "(shows|artists|venues)" family/)
      expect(captureException).toHaveBeenCalled()
    })

    // A malformed ROW is the same failure at finer granularity: silently
    // filtering it away would publish a sitemap short a URL with no signal.
    // It is rejected inside the try, so it keeps the tagged Sentry capture.
    it.each([
      ['a null row', [null, { slug: 'real', updated_at: ISO }]],
      ['a row with no slug key', [{ updated_at: ISO }]],
      ['a row whose slug is not a string', [{ slug: 42, updated_at: ISO }]],
    ])('throws on %s', async (_label, shows) => {
      vi.stubGlobal('fetch', respondWith({ shows, artists: [], venues: [] }))

      await expect(sitemap()).rejects.toThrow(/malformed row in the "shows" family/)
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
