import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException, captureMessage } = vi.hoisted(() => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
  captureMessage,
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

import sitemap, { generateSitemaps } from './sitemap'
import {
  ALL_SHARD_IDS,
  ARTIST_SHARD_IDS,
  ENTITY_SHARD_IDS,
  RELEASE_SHARD_IDS,
  SHOW_SHARD_IDS,
  shardFamily,
} from './sitemap-shards'

/** The artists sub-shard the artists cases drive. */
const [ARTIST_SHARD] = ARTIST_SHARD_IDS

/** The releases sub-shard the sub-sharding cases drive. */
const [RELEASE_SHARD] = RELEASE_SHARD_IDS

/** The shows sub-shard the shows cases drive. */
const [SHOW_SHARD] = SHOW_SHARD_IDS

const ISO = '2026-07-20T12:00:00Z'

function emptyFamilies(
  overrides: Partial<Record<string, unknown>> = {}
): Record<string, unknown> {
  return {
    shows: [],
    artists: [],
    venues: [],
    venue_years: [],
    scenes: [],
    scene_weeks: [],
    labels: [],
    releases: [],
    festivals: [],
    tags: [],
    ...overrides,
  }
}

/**
 * A fetch stub answering every call with the same payload.
 *
 * A NEW Response per call, not one shared instance: a body can only be read
 * once, so a shared Response makes the second shard of a multi-shard test fail
 * with "Body is unusable" — which reads as a bug in the generator rather than
 * in the fixture.
 */
function respondWith(body: unknown, status = 200) {
  // Typed with fetch's signature rather than taking an ignored parameter, so
  // `mock.calls` carries the requested URL for the tests that assert WHICH
  // shard was fetched, without an unused binding for the linter to flag.
  return vi.fn<(input: RequestInfo | URL) => Promise<Response>>(
    async () =>
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      })
  )
}

async function urlsOf(id: string) {
  const entries = await sitemap({ id: Promise.resolve(id) })
  return entries.map(e => e.url)
}

describe('sitemap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  /**
   * Driven from the shared shard list rather than a hand-written enumeration.
   * The hand-written one silently skipped `venue_years` when it was added
   * (PSY-1756) while still claiming to cover "every entity family"; a list this
   * function is itself built from cannot.
   */
  it('generateSitemaps lists the pages shard plus every entity shard', async () => {
    const ids = (await generateSitemaps()).map(s => s.id)
    expect(ids).toEqual([...ALL_SHARD_IDS])
  })

  it('maps every entity shard onto its family URL prefix', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      // The query value is the SHARD id; the response is keyed by the FAMILY
      // it slices. Resolving through the shared table here is what makes the
      // sub-shard cases below exercise the real mapping rather than a fixture
      // that happens to agree with it.
      const shard = new URL(url).searchParams.get('family')!
      const family = shardFamily(shard)!
      const body: Record<string, unknown> = emptyFamilies({
        [family]: [{ slug: `a-${family}`, updated_at: ISO }],
      })
      // The composite-slug families carry a whole path tail as their slug.
      if (family === 'scene_weeks') {
        body.scene_weeks = [{ slug: 'phoenix-az/2026-W31', updated_at: ISO }]
      }
      if (family === 'venue_years') {
        body.venue_years = [
          { slug: 'the-van-buren/shows/2025', updated_at: ISO },
        ]
      }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    expect(await urlsOf(SHOW_SHARD)).toContain('https://psychichomily.com/shows/a-shows')
    expect(await urlsOf(ARTIST_SHARD)).toContain(
      'https://psychichomily.com/artists/a-artists'
    )
    expect(await urlsOf('venues')).toContain(
      'https://psychichomily.com/venues/a-venues'
    )
    expect(await urlsOf('venue_years')).toContain(
      'https://psychichomily.com/venues/the-van-buren/shows/2025'
    )
    expect(await urlsOf('scenes')).toContain(
      'https://psychichomily.com/scenes/a-scenes'
    )
    expect(await urlsOf('scene_weeks')).toContain(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
    expect(await urlsOf('labels')).toContain(
      'https://psychichomily.com/labels/a-labels'
    )
    // Every releases sub-shard, and all under the SAME prefix: sub-sharding is
    // a transport detail that must not reach the emitted URLs.
    for (const shard of RELEASE_SHARD_IDS) {
      expect(await urlsOf(shard)).toContain(
        'https://psychichomily.com/releases/a-releases'
      )
    }
    expect(await urlsOf('festivals')).toContain(
      'https://psychichomily.com/festivals/a-festivals'
    )
    expect(await urlsOf('tags')).toContain('https://psychichomily.com/tags/a-tags')
  })

  it('carries updated_at through to lastModified', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith(emptyFamilies({ shows: [{ slug: 'a-show', updated_at: ISO }] }))
    )

    const entry = (await sitemap({ id: Promise.resolve(SHOW_SHARD) })).find(
      e => e.url === 'https://psychichomily.com/shows/a-show'
    )

    expect(entry?.lastModified).toEqual(new Date(ISO))
  })

  // The old generator stamped every static entry with `new Date()` on each
  // render, telling crawlers the whole site changed every time. That devalues
  // <lastmod> for the entries that genuinely did change.
  it('does not stamp static pages with a render-time lastModified', async () => {
    const home = (await sitemap({ id: Promise.resolve('pages') })).find(
      e => e.url === 'https://psychichomily.com'
    )

    expect(home).toBeDefined()
    expect(home?.lastModified).toBeUndefined()
  })

  it('includes /scenes and the other list pages on the pages shard', async () => {
    const urls = await urlsOf('pages')
    expect(urls).toContain('https://psychichomily.com/scenes')
    expect(urls).toContain('https://psychichomily.com/labels')
    expect(urls).toContain('https://psychichomily.com/releases')
    expect(urls).toContain('https://psychichomily.com/festivals')
    expect(urls).toContain('https://psychichomily.com/tags')
  })

  it('keeps an entry but omits lastModified when updated_at is unparseable', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith(
        emptyFamilies({ shows: [{ slug: 'a-show', updated_at: 'not-a-date' }] })
      )
    )

    const entry = (await sitemap({ id: Promise.resolve(SHOW_SHARD) })).find(
      e => e.url === 'https://psychichomily.com/shows/a-show'
    )

    expect(entry).toBeDefined()
    expect(entry?.lastModified).toBeUndefined()
  })

  it('skips entries with no slug — they have no canonical URL', async () => {
    vi.stubGlobal(
      'fetch',
      respondWith(
        emptyFamilies({
          shows: [
            { slug: '', updated_at: ISO },
            { slug: 'real', updated_at: ISO },
          ],
        })
      )
    )

    const showUrls = (await urlsOf(SHOW_SHARD)).filter(u =>
      u.startsWith('https://psychichomily.com/shows/')
    )

    expect(showUrls).toEqual(['https://psychichomily.com/shows/real'])
  })

  it('carries blog frontmatter dates through, and omits lastModified when absent', async () => {
    const entries = await sitemap({ id: Promise.resolve('pages') })
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
  describe('fails closed', () => {
    it('throws when the feed errors rather than emitting a partial sitemap', async () => {
      vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')))

      await expect(sitemap({ id: Promise.resolve(SHOW_SHARD) })).rejects.toThrow(
        'network down'
      )
      expect(captureException).toHaveBeenCalled()
    })

    it('throws on a non-ok response', async () => {
      vi.stubGlobal('fetch', respondWith({}, 500))

      await expect(sitemap({ id: Promise.resolve(SHOW_SHARD) })).rejects.toThrow(/500/)
      expect(captureException).toHaveBeenCalled()
    })

    it.each([
      ['null', emptyFamilies({ shows: null })],
      ['absent', (() => {
        const body = emptyFamilies()
        delete (body as { shows?: unknown }).shows
        return body
      })()],
      ['not an array', emptyFamilies({ shows: {} })],
      ['an empty body', {}],
    ])('throws when a family is %s', async (_label, body) => {
      vi.stubGlobal('fetch', respondWith(body))

      await expect(sitemap({ id: Promise.resolve(SHOW_SHARD) })).rejects.toThrow(
        /missing the "shows" family/
      )
      expect(captureException).toHaveBeenCalled()
    })

    it.each([
      ['a null row', [null, { slug: 'real', updated_at: ISO }]],
      ['a row with no slug key', [{ updated_at: ISO }]],
      ['a row whose slug is not a string', [{ slug: 42, updated_at: ISO }]],
    ])('throws on %s', async (_label, shows) => {
      vi.stubGlobal('fetch', respondWith(emptyFamilies({ shows })))

      await expect(sitemap({ id: Promise.resolve(SHOW_SHARD) })).rejects.toThrow(
        /malformed row in the "shows" family/
      )
      expect(captureException).toHaveBeenCalled()
    })
  })

  /**
   * The one non-failure that yields an empty document (PSY-1756).
   *
   * A frontend that ships a new family before the backend implementing it gets
   * a definite "I do not serve that" — HTTP 422 from huma's enum, or 400 from
   * the service's own guard. Measured against the deployed stage backend during
   * this ticket: every preview build failed the prerender gate on that single
   * shard while the other ten prerendered from the same healthy backend.
   *
   * The document has to be EMPTY and VALID, not absent: an absent one leaves the
   * shard Dynamic, which is the outage exposure the gate refuses.
   */
  describe('degrades a family the backend does not serve', () => {
    it.each([422, 400])('renders an empty shard on HTTP %d', async status => {
      vi.stubGlobal('fetch', respondWith({ detail: 'validation failed' }, status))

      await expect(urlsOf('venue_years')).resolves.toEqual([])
    })

    it('warns rather than erroring, and names the family', async () => {
      vi.stubGlobal('fetch', respondWith({}, 422))

      await urlsOf('venue_years')

      expect(captureMessage).toHaveBeenCalledWith(
        expect.stringContaining('venue_years'),
        expect.objectContaining({ level: 'warning' })
      )
      // A degraded family is not an error: an error here would page someone for
      // an expected state that self-heals on the next build.
      expect(captureException).not.toHaveBeenCalled()
    })

    /**
     * The boundary that keeps this from becoming the incident it mitigates. A
     * moved or misconfigured `/sitemap/entries` answers 404 for EVERY family,
     * so degrading on it would publish an empty document for the whole
     * catalogue rather than failing the build.
     */
    it('still throws on 404 — a moved endpoint must not empty every family', async () => {
      vi.stubGlobal('fetch', respondWith({}, 404))

      await expect(sitemap({ id: Promise.resolve('venue_years') })).rejects.toThrow(
        /404/
      )
      expect(captureException).toHaveBeenCalled()
    })

    it.each([500, 503])('still throws on HTTP %d', async status => {
      vi.stubGlobal('fetch', respondWith({}, status))

      await expect(sitemap({ id: Promise.resolve('venue_years') })).rejects.toThrow()
      expect(captureException).toHaveBeenCalled()
    })

    it('still throws when the backend is unreachable', async () => {
      vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')))

      await expect(sitemap({ id: Promise.resolve('venue_years') })).rejects.toThrow(
        'network down'
      )
    })

    /**
     * A 200 that simply omits the family is the incident signature — a family
     * silently missing from a successful-looking response — and stays fatal.
     * Only an explicit "I do not serve that" degrades.
     */
    it('still throws on a 200 whose family key is absent', async () => {
      const body = emptyFamilies()
      delete (body as { venue_years?: unknown }).venue_years
      vi.stubGlobal('fetch', respondWith(body))

      await expect(sitemap({ id: Promise.resolve('venue_years') })).rejects.toThrow(
        /missing the "venue_years" family/
      )
    })

    /** The degrade applies to whichever family the backend rejects, not a list. */
    it('is not special-cased to venue_years', async () => {
      vi.stubGlobal('fetch', respondWith({}, 422))

      await expect(urlsOf('festivals')).resolves.toEqual([])
    })
  })

  it('requests the projection feed scoped to the shard', async () => {
    const fetchMock = respondWith(emptyFamilies())
    vi.stubGlobal('fetch', fetchMock)

    await sitemap({ id: Promise.resolve(SHOW_SHARD) })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(new RegExp(`/sitemap/entries\\?family=${SHOW_SHARD}$`)),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('does not fetch the backend for the pages shard', async () => {
    const fetchMock = respondWith(emptyFamilies())
    vi.stubGlobal('fetch', fetchMock)

    await sitemap({ id: Promise.resolve('pages') })

    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('throws on a shard id nothing serves', async () => {
    // The `id` reaching this route comes from generateSitemaps(), so an
    // unrecognised one means the route table and the shard table have drifted.
    // Rendering an empty document instead would publish a valid-looking sitemap
    // that is missing a whole shard.
    await expect(sitemap({ id: Promise.resolve('releases-nope') })).rejects.toThrow(
      /unknown sitemap shard id/
    )
  })

  /**
   * The releases family outgrew a single Data Cache entry and is served in slug
   * ranges (PSY-1763). What has to hold for that to be invisible from outside is
   * that each range reads its rows out of the FAMILY's key — the other half,
   * that each asks for its own id, is covered once for every shard below.
   */
  describe('releases sub-shards', () => {
    it('reads its rows from the releases key of the response', async () => {
      vi.stubGlobal(
        'fetch',
        respondWith(
          emptyFamilies({ releases: [{ slug: 'a-record', updated_at: ISO }] })
        )
      )

      await expect(urlsOf(RELEASE_SHARD)).resolves.toEqual([
        'https://psychichomily.com/releases/a-record',
      ])
    })

    /**
     * The deploy-race path, which is why the sub-shard rides in the `family`
     * parameter at all: a backend that predates these ids rejects them with the
     * same 400/422 it uses for an unknown family, so a sub-shard degrades to an
     * empty document for one window instead of failing the build.
     */
    it.each([422, 400])('degrades to an empty document on HTTP %d', async status => {
      vi.stubGlobal('fetch', respondWith({ detail: 'validation failed' }, status))

      await expect(urlsOf(RELEASE_SHARD)).resolves.toEqual([])
    })

    it('still fails closed when the releases key is absent', async () => {
      const body = emptyFamilies()
      delete (body as { releases?: unknown }).releases
      vi.stubGlobal('fetch', respondWith(body))

      await expect(sitemap({ id: Promise.resolve(RELEASE_SHARD) })).rejects.toThrow(
        /missing the "releases" family/
      )
    })

  })

  /**
   * One fetch per shard, each carrying the shard's OWN id — which is the whole
   * point for the releases ranges: four documents, four independently-cached
   * fetches, so the over-cap payload never lands back in one entry. Driven from
   * ENTITY_SHARD_IDS rather than a release-specific list because that list
   * CONTAINS the release ranges, so a separate release-only case would assert a
   * strict subset of this.
   */
  it('fetches exactly one feed per entity shard, keyed by shard id, none twice', async () => {
    const fetchMock = respondWith(emptyFamilies())
    vi.stubGlobal('fetch', fetchMock)

    for (const id of ENTITY_SHARD_IDS) {
      await sitemap({ id: Promise.resolve(id) })
    }

    const requested = fetchMock.mock.calls.map(([input]) =>
      new URL(String(input)).searchParams.get('family')
    )
    expect(requested).toEqual([...ENTITY_SHARD_IDS])
  })
})
