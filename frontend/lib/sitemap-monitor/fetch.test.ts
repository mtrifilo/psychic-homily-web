import { afterEach, describe, expect, it, vi } from 'vitest'
import { FAMILY_SHARD_IDS, PAGES_SHARD_ID } from '@/app/sitemap-shards'
import { resolveConfig } from './config'
import { fetchExpectedCounts, rebaseOnTarget, walkSitemap } from './fetch'

const STAGE = 'https://stage.psychichomily.com'

/** Retry delay 0 — the real 5s backoff would add 5s to every failure-path test. */
function testConfig(env: Record<string, string> = {}) {
  return resolveConfig({ SITEMAP_MONITOR_RETRY_DELAY_MS: '0', ...env })
}

/**
 * The index served by every environment hardcodes the PRODUCTION base URL, so
 * a stage run must rewrite the host before following the links. Without this
 * the monitor would fetch production while reporting the stage target.
 */
describe('rebaseOnTarget', () => {
  it('rewrites a production-hosted shard loc onto the target origin', () => {
    expect(rebaseOnTarget('https://psychichomily.com/sitemap/shows.xml', STAGE)).toBe(
      `${STAGE}/sitemap/shows.xml`
    )
  })

  it('leaves a loc already on the target untouched', () => {
    expect(rebaseOnTarget(`${STAGE}/sitemap/shows.xml`, STAGE)).toBe(
      `${STAGE}/sitemap/shows.xml`
    )
  })

  it('rebases onto an unrelated origin', () => {
    expect(rebaseOnTarget('https://psychichomily.com/sitemap/tags.xml', 'https://example.com')).toBe(
      'https://example.com/sitemap/tags.xml'
    )
  })

  /**
   * The anchor must not be walkable off the target. A pathname beginning `//`
   * is a SCHEME-RELATIVE reference, so resolving it against the origin string
   * would yield `https://evil.com/...` — defeating the one control that keeps
   * the bypass header on first-party hosts.
   */
  it('does not let a protocol-relative path escape the target origin', () => {
    expect(
      rebaseOnTarget('https://psychichomily.com//evil.com/sitemap.xml', STAGE)
    ).toBe(`${STAGE}//evil.com/sitemap.xml`)
    expect(new URL(rebaseOnTarget('https://x.com//evil.com/a.xml', STAGE)).origin).toBe(STAGE)
  })
})

function xmlResponse(body: string, status = 200): Response {
  return new Response(body, { status, headers: { 'content-type': 'application/xml' } })
}

function urlset(locs: string[]): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${locs.map(loc => `<url><loc>${loc}</loc></url>`).join('\n')}
</urlset>`
}

function index(ids: string[]): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${ids
  .map(id => `  <sitemap><loc>https://psychichomily.com/sitemap/${id}.xml</loc></sitemap>`)
  .join('\n')}
</sitemapindex>`
}

// Derived, not hand-copied: a family added to sitemap-shards.ts must flow into
// these fixtures, or the shard-count assertions below would keep passing for
// the wrong reason.
const ALL_IDS: string[] = [PAGES_SHARD_ID, ...FAMILY_SHARD_IDS]

afterEach(() => {
  vi.restoreAllMocks()
})

describe('walkSitemap', () => {
  it('sums per-family counts across every shard of an index', async () => {
    const bodies: Record<string, string> = {
      [`${STAGE}/sitemap-index`]: index(ALL_IDS),
      [`${STAGE}/sitemap/pages.xml`]: urlset(['https://psychichomily.com/']),
      [`${STAGE}/sitemap/shows.xml`]: urlset([
        'https://psychichomily.com/shows/2026-08-01-a',
        'https://psychichomily.com/shows/2025-01-01-b',
      ]),
      [`${STAGE}/sitemap/artists.xml`]: urlset(['https://psychichomily.com/artists/a']),
    }
    for (const id of ALL_IDS) {
      bodies[`${STAGE}/sitemap/${id}.xml`] ??= urlset([])
    }

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => xmlResponse(bodies[url] ?? '<urlset></urlset>'))
    )

    const observation = await walkSitemap(
      testConfig({ SITEMAP_MONITOR_TARGET: STAGE })
    )

    expect(observation.shape).toBe('index')
    expect(observation.shardCount).toBe(ALL_IDS.length)
    expect(observation.observedByFamily.shows).toBe(2)
    expect(observation.observedByFamily.artists).toBe(1)
    expect(observation.observedPages).toBe(1)
    expect(observation.showDates).toEqual(['2026-08-01', '2025-01-01'])
    expect(observation.errors).toEqual([])
  })

  /**
   * Every collected loc must be anchored to the target, not to the production
   * host baked into the document. Otherwise a stage run probes production —
   * and does so carrying the Vercel bypass token.
   */
  it('anchors collected locs to the target origin, not the document host', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        xmlResponse(
          url.endsWith('/sitemap-index')
            ? index(ALL_IDS)
            : urlset(['https://psychichomily.com/artists/a'])
        )
      )
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))
    const everyLoc = [...observation.locsByBucket.values()].flat()
    expect(everyLoc.length).toBeGreaterThan(0)
    for (const loc of everyLoc) {
      expect(new URL(loc).origin).toBe(STAGE)
    }
  })

  // The whole point of walking the index: fetching only the entry document
  // would report a near-empty catalogue.
  it('fetches every shard rather than the index alone', async () => {
    const spy = vi.fn(async (url: string) =>
      xmlResponse(url.endsWith('/sitemap-index') ? index(ALL_IDS) : urlset([]))
    )
    vi.stubGlobal('fetch', spy)

    await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))

    // 1 index + every shard.
    expect(spy).toHaveBeenCalledTimes(ALL_IDS.length + 1)
    for (const id of ALL_IDS) {
      expect(spy).toHaveBeenCalledWith(`${STAGE}/sitemap/${id}.xml`, expect.anything())
    }
  })

  it('records an error when a family shard is absent from the index', async () => {
    const present = ALL_IDS.filter(id => id !== 'releases')
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        xmlResponse(url.endsWith('/sitemap-index') ? index(present) : urlset([]))
      )
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))
    expect(observation.errors).toContain('sitemap index is missing the "releases" shard')
  })

  it('records an error when a shard fails to fetch, and keeps going', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/sitemap-index')) return xmlResponse(index(ALL_IDS))
        if (url.endsWith('/shows.xml')) return xmlResponse('server error', 503)
        return xmlResponse(urlset(['https://psychichomily.com/artists/a']))
      })
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))
    expect(observation.errors.some(e => e.includes('shard "shows"') && e.includes('503'))).toBe(
      true
    )
    expect(observation.observedByFamily.artists).toBe(1)
  })

  // Production served this shape until the sharding deployed; the monitor has
  // to keep working across that boundary in both directions.
  it('classifies a single-document sitemap by URL path', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        xmlResponse(
          urlset([
            'https://psychichomily.com/',
            'https://psychichomily.com/shows',
            'https://psychichomily.com/shows/2026-08-01-a',
            'https://psychichomily.com/artists/a',
            'https://psychichomily.com/scenes/austin-tx',
            'https://psychichomily.com/scenes/austin-tx/2026-W28',
          ])
        )
      )
    )

    const observation = await walkSitemap(testConfig())
    expect(observation.shape).toBe('urlset')
    expect(observation.shardCount).toBe(0)
    expect(observation.observedByFamily.shows).toBe(1)
    expect(observation.observedByFamily.artists).toBe(1)
    expect(observation.observedByFamily.scenes).toBe(1)
    expect(observation.observedByFamily.scene_weeks).toBe(1)
    expect(observation.observedPages).toBe(2)
    expect(observation.showDates).toEqual(['2026-08-01'])
  })

  it('throws when the entry point is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => xmlResponse('nope', 500)))
    await expect(walkSitemap(testConfig())).rejects.toThrow(/returned 500/)
  })
})

describe('fetchExpectedCounts', () => {
  function entriesBody(overrides: Record<string, unknown> = {}) {
    const base = Object.fromEntries(FAMILY_SHARD_IDS.map(family => [family, []]))
    return JSON.stringify({ ...base, ...overrides })
  }

  it('counts the rows of each family', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            entriesBody({
              shows: [{ slug: 'a' }, { slug: 'b' }],
              artists: [{ slug: 'c' }],
            })
          )
      )
    )

    const counts = await fetchExpectedCounts(testConfig())
    expect(counts.shows).toBe(2)
    expect(counts.artists).toBe(1)
    expect(counts.venues).toBe(0)
  })

  // app/sitemap.ts drops empty slugs, so counting them here would manufacture
  // drift that does not exist.
  it('ignores rows the sitemap would not emit', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(entriesBody({ shows: [{ slug: 'a' }, { slug: '' }, { notASlug: 1 }] }))
      )
    )
    expect((await fetchExpectedCounts(testConfig())).shows).toBe(1)
  })

  // Coercing a missing family to zero would blame the sitemap for an API fault.
  it('throws when a family is missing from the response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ shows: [] })))
    )
    await expect(fetchExpectedCounts(testConfig())).rejects.toThrow(
      /missing the "artists" family/
    )
  })

  it('throws when the endpoint is unavailable', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('404 page not found', { status: 404 })))
    await expect(fetchExpectedCounts(testConfig())).rejects.toThrow(/returned 404/)
  })
})
