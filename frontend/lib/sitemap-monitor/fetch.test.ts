import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ENTITY_SHARD_IDS,
  PAGES_SHARD_ID,
  RELEASE_SHARD_IDS,
  SHOW_SHARD_IDS,
  shardFamily,
  SITEMAP_FAMILIES,
} from '@/app/sitemap-shards'
import { resolveConfig } from './config'
import {
  fetchExpectedCounts,
  rebaseOnTarget,
  sampleUrls,
  SHARD_FETCH_CONCURRENCY,
  walkSitemap,
} from './fetch'

const STAGE = 'https://stage.psychichomily.com'

/** The shows sub-shard the shows-family fixtures below serve rows from. */
const [SHOW_SHARD] = SHOW_SHARD_IDS

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
const ALL_IDS: string[] = [PAGES_SHARD_ID, ...ENTITY_SHARD_IDS]

afterEach(() => {
  vi.restoreAllMocks()
})

describe('walkSitemap', () => {
  it('sums per-family counts across every shard of an index', async () => {
    const bodies: Record<string, string> = {
      [`${STAGE}/sitemap-index`]: index(ALL_IDS),
      [`${STAGE}/sitemap/pages.xml`]: urlset(['https://psychichomily.com/']),
      [`${STAGE}/sitemap/${SHOW_SHARD}.xml`]: urlset([
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

  /**
   * The shards are fetched concurrently, and both halves of that are
   * load-bearing.
   *
   * The BOUND is what keeps the walk off the origin's anonymous per-IP rate
   * limiter: unbounded, a 40-shard index would open 40 sockets at once and the
   * monitor would be measuring the limiter. The ORDER is what keeps the report
   * diffable — `errors`, `showDates` and `locsByBucket` are order-dependent
   * accumulators, so folding results as they land would make two runs over an
   * identical sitemap print different documents.
   */
  it('fetches shards with bounded concurrency and folds them in index order', async () => {
    let inFlight = 0
    let peak = 0
    const release: Array<() => void> = []

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/sitemap-index')) return xmlResponse(index(ALL_IDS))
        inFlight++
        peak = Math.max(peak, inFlight)
        // Hold every shard open until the whole first wave has arrived, so the
        // peak is a real observation rather than a scheduling accident. The
        // LAST-listed shard resolves first, which is what makes the ordering
        // assertion below meaningful.
        await new Promise<void>(resolve => {
          release.push(resolve)
          if (release.length >= SHARD_FETCH_CONCURRENCY) release.reverse().forEach(r => r())
        })
        inFlight--
        return xmlResponse('not a sitemap document', 500)
      })
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))

    expect(peak).toBeGreaterThan(1)
    expect(peak).toBeLessThanOrEqual(SHARD_FETCH_CONCURRENCY)
    // Every shard failed, so the errors are one per shard and must appear in
    // the order the index listed them, not the order the fetches settled.
    expect(observation.errors).toEqual(
      ALL_IDS.map(id => expect.stringContaining(`shard "${id}"`))
    )
  })

  /**
   * PARSING failures stay per-shard too, not just fetch failures.
   *
   * Both inputs below reach a throw AFTER the fetch succeeded: `fetchDocument`
   * checks only `response.ok`, so a 200 carrying an edge interstitial reaches
   * `detectShape`, and a `<loc>` that is not a URL reaches `new URL` inside
   * `rebaseOnTarget`. If either escaped, the run would lose every other shard's
   * observations and report a crash naming no shard at all.
   */
  it.each([
    ['a 200 that is not a sitemap document', '<html><body>gateway</body></html>'],
    ['a malformed loc', `<urlset><url><loc>not a url</loc></url></urlset>`],
  ])('keeps going when one shard serves %s', async (_label, body) => {
    const [broken] = ENTITY_SHARD_IDS
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/sitemap-index')) return xmlResponse(index(ALL_IDS))
        if (url.endsWith(`/${broken}.xml`)) return xmlResponse(body)
        return xmlResponse(urlset(['https://psychichomily.com/artists/a']))
      })
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))

    expect(observation.errors.some(e => e.includes(`shard "${broken}"`))).toBe(true)
    // The rest of the walk still happened, which is what makes this a per-shard
    // boundary rather than a caught-and-abandoned run.
    expect(observation.observedByFamily.artists).toBeGreaterThan(0)
  })

  /**
   * Per SHARD, not per family. A sub-sharded family loses only a fraction of
   * its URLs when one of its documents goes missing — well inside the
   * per-family drift tolerance — so a family-level check would pass while a
   * quarter of the release catalogue quietly left the index.
   */
  it.each(ENTITY_SHARD_IDS)('records an error when the %s shard is absent from the index', async missing => {
    const present = ALL_IDS.filter(id => id !== missing)
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        xmlResponse(url.endsWith('/sitemap-index') ? index(present) : urlset([]))
      )
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))
    expect(observation.errors).toContain(`sitemap index is missing the "${missing}" shard`)
  })

  /**
   * The other half of the same hole, and the one the per-shard membership check
   * above cannot see: a shard that IS listed but serves an empty document.
   * A generator fetch that 400/422s degrades to an empty-but-valid <urlset>, so
   * a range can go dark while its siblings are healthy — and neither the
   * membership check (the shard is present) nor `vanished` (the family is not
   * at zero) fires. All that would be left is drift, which one range of four may
   * or may not clear depending on where the cut points fell.
   *
   * Scoped to SIBLINGS rather than to the API's expected counts, which
   * walkSitemap does not have. The second case pins that scoping as deliberate:
   * a whole family at zero must stay SILENT here, because that is `vanished`'s
   * job and double-reporting it would train the reader to ignore both.
   */
  describe('a listed shard that serves nothing', () => {
    const [darkShard, ...litShards] = RELEASE_SHARD_IDS

    function serveWith(emptyIds: readonly string[]) {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string) => {
          if (url.endsWith('/sitemap-index')) return xmlResponse(index(ALL_IDS))
          const id = url.split('/sitemap/')[1]?.replace('.xml', '') ?? ''
          // Every non-releases shard serves a URL so the releases ranges are
          // the only thing under test.
          const isEmpty = emptyIds.includes(id)
          return xmlResponse(
            urlset(isEmpty ? [] : [`https://psychichomily.com/releases/${id}-one`])
          )
        })
      )
    }

    const emptyShardError = (id: string) =>
      `shard "${id}" served an empty document while other shards of the ` +
      `"releases" family served URLs — that range is not being announced`

    it('is reported when its siblings served URLs', async () => {
      serveWith([darkShard])

      const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))

      expect(observation.errors).toContain(emptyShardError(darkShard))
      // Only the dark one — a healthy sibling must not be reported too.
      for (const lit of litShards) {
        expect(observation.errors).not.toContain(emptyShardError(lit))
      }
    })

    it('is NOT reported when the whole family is empty — that is `vanished`', async () => {
      serveWith(RELEASE_SHARD_IDS)

      const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))

      for (const id of RELEASE_SHARD_IDS) {
        expect(observation.errors).not.toContain(emptyShardError(id))
      }
    })

    /**
     * The shows shards are calendar months, so an empty one is a month nobody
     * has booked yet rather than a lost document — subShardsCanBeEmpty.
     * Reported, this would fire on almost every run: the enumerated span runs
     * to the end of the year after next.
     */
    it('is NOT reported for a family whose ranges are legitimately sparse', async () => {
      const showShards: readonly string[] = SHOW_SHARD_IDS
      const [darkMonth, ...litMonths] = showShards
      expect(litMonths.length).toBeGreaterThan(0)

      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string) => {
          if (url.endsWith('/sitemap-index')) return xmlResponse(index(ALL_IDS))
          const id = url.split('/sitemap/')[1]?.replace('.xml', '') ?? ''
          return xmlResponse(
            urlset(id === darkMonth ? [] : [`https://psychichomily.com/shows/${id}-one`])
          )
        })
      )

      const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))

      expect(observation.errors).toEqual([])
    })
  })

  it('records an error when a shard fails to fetch, and keeps going', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/sitemap-index')) return xmlResponse(index(ALL_IDS))
        if (url.endsWith(`/${SHOW_SHARD}.xml`)) return xmlResponse('server error', 503)
        return xmlResponse(urlset(['https://psychichomily.com/artists/a']))
      })
    )

    const observation = await walkSitemap(testConfig({ SITEMAP_MONITOR_TARGET: STAGE }))
    expect(observation.errors.some(e => e.includes(`shard "${SHOW_SHARD}"`) && e.includes('503'))).toBe(
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
    // Keyed by FAMILY: this stands in for the unsharded `/sitemap/entries`
    // response, whose keys are the schema's families whatever the sitemap
    // chooses to shard them into.
    const base = Object.fromEntries(SITEMAP_FAMILIES.map(family => [family, []]))
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

  // A routine backend redeploy landing on the cron must not alarm.
  it('retries a 5xx once and succeeds on the second attempt', async () => {
    let calls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        calls++
        if (calls === 1) return new Response('bad gateway', { status: 502 })
        return new Response(entriesBody({ shows: [{ slug: 'a' }] }))
      })
    )

    expect((await fetchExpectedCounts(testConfig())).shows).toBe(1)
    expect(calls).toBe(2)
  })

  // A 404 is a stable answer; retrying only doubles the time to the same verdict.
  it('does not retry a 4xx', async () => {
    let calls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        calls++
        return new Response('nope', { status: 404 })
      })
    )

    await expect(fetchExpectedCounts(testConfig())).rejects.toThrow(/returned 404/)
    expect(calls).toBe(1)
  })
})

/**
 * The probe path issues most of the monitor's requests — ten dynamically
 * rendered pages at the cron instant — so its retry behaviour is what decides
 * whether a routine redeploy produces a false alarm.
 */
describe('sampleUrls', () => {
  const TARGET = 'https://psychichomily.com'

  it('reports a reachable URL', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('ok')))
    const [result] = await sampleUrls([`${TARGET}/shows/a`], testConfig())
    expect(result.ok).toBe(true)
    expect(result.status).toBe(200)
  })

  // A 5xx is RETURNED rather than thrown, so retrying the transport error alone
  // would have skipped the far more likely case.
  it('retries a 5xx probe and reports the recovered status', async () => {
    let calls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        calls++
        return calls === 1 ? new Response('bad gateway', { status: 502 }) : new Response('ok')
      })
    )

    const [result] = await sampleUrls([`${TARGET}/shows/a`], testConfig())
    expect(calls).toBe(2)
    expect(result.ok).toBe(true)
  })

  // The finding itself must not be retried away or softened.
  it('reports a 404 without retrying', async () => {
    let calls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        calls++
        return new Response('gone', { status: 404 })
      })
    )

    const [result] = await sampleUrls([`${TARGET}/shows/gone`], testConfig())
    expect(calls).toBe(1)
    expect(result.ok).toBe(false)
    expect(result.status).toBe(404)
  })

  // The bypass token must never reach a host the document named.
  it('refuses to probe an off-target origin', async () => {
    const spy = vi.fn(async () => new Response('ok'))
    vi.stubGlobal('fetch', spy)

    const [result] = await sampleUrls(['https://evil.com/shows/a'], testConfig())
    expect(result.ok).toBe(false)
    expect(result.error).toMatch(/off-target origin/)
    expect(spy).not.toHaveBeenCalled()
  })
})
