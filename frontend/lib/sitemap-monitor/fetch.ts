/**
 * The I/O half of the monitor: gather what production is actually serving.
 *
 * Everything here talks to the network and returns raw observations; the
 * verdict is evaluate.ts's job.
 */

import { FAMILY_SHARD_IDS, PAGES_SHARD_ID, type Family } from '@/app/sitemap-shards'
import type { MonitorConfig } from './config'
import {
  classifyLoc,
  detectShape,
  parseSitemapIndex,
  parseUrlset,
  showDateFromLoc,
  type LocBucket,
  type SitemapShape,
} from './parse'
import type { SampleResult } from './evaluate'

/** Everything read off the served sitemap, before any judgement. */
export interface SitemapObservation {
  shape: SitemapShape
  shardCount: number
  observedByFamily: Record<Family, number>
  observedPages: number
  observedOther: number
  /** Every `<loc>` seen, for sampling. */
  locs: string[]
  /** Slug dates of every show URL that carries one. */
  showDates: string[]
  errors: string[]
}

const USER_AGENT = 'psychic-homily-sitemap-monitor'

function emptyCounts(): Record<Family, number> {
  return Object.fromEntries(FAMILY_SHARD_IDS.map(f => [f, 0])) as Record<Family, number>
}

function headers(config: MonitorConfig): Record<string, string> {
  // A plain UA: some edges serve bot-flavoured responses to the default fetch
  // agent, which would make the monitor measure something no crawler sees.
  const value: Record<string, string> = { 'user-agent': USER_AGENT }
  if (config.vercelBypassToken) {
    value['x-vercel-protection-bypass'] = config.vercelBypassToken
  }
  return value
}

/** GET a URL, failing on any non-2xx so a served error page never parses as data. */
async function request(url: string, config: MonitorConfig): Promise<Response> {
  const response = await fetch(url, {
    headers: headers(config),
    redirect: 'follow',
    signal: AbortSignal.timeout(config.fetchTimeoutMs),
  })
  if (!response.ok) {
    throw new Error(`GET ${url} returned ${response.status}`)
  }
  return response
}

/**
 * Rewrite a child-document URL onto the origin being checked.
 *
 * NOT cosmetic. app/sitemap-index/route.ts hardcodes the production base URL in
 * every `<loc>`, so the index served by stage — or by a preview deployment —
 * lists `https://psychichomily.com/sitemap/shows.xml`. Following those links
 * verbatim would silently measure PRODUCTION while reporting the stage target,
 * which is worse than not checking at all. Verified against stage on
 * 2026-07-30.
 */
export function rebaseOnTarget(loc: string, target: string): string {
  const source = new URL(loc)
  const base = new URL(target)
  return new URL(`${source.pathname}${source.search}`, base.origin).toString()
}

function shardIdFromUrl(url: string): string {
  const path = new URL(url).pathname
  return path.slice(path.lastIndexOf('/') + 1).replace(/\.xml$/, '')
}

function countInto(observation: SitemapObservation, bucket: LocBucket, count: number): void {
  if (bucket === PAGES_SHARD_ID) observation.observedPages += count
  else if (bucket === 'other') observation.observedOther += count
  else observation.observedByFamily[bucket] += count
}

function collectShowDates(locs: readonly string[], into: string[]): void {
  for (const loc of locs) {
    const date = showDateFromLoc(loc)
    if (date) into.push(date)
  }
}

/**
 * Read the sitemap the way a crawler would.
 *
 * Handles BOTH document shapes on purpose. Production served a single
 * `<urlset>` before the generateSitemaps() sharding shipped and a
 * `<sitemapindex>` after; a monitor that understood only one of them would be
 * broken across that deploy in one direction or the other, and "the monitor
 * was broken" is how the original defect survived. The shape actually served
 * is reported so a silent regression from index back to single-document is
 * visible rather than merely tolerated.
 *
 * When an index is served the shard id IS the family, so the family counts
 * cannot be skewed by a URL-classification gap. classifyLoc is only used for
 * the single-document shape.
 */
export async function walkSitemap(config: MonitorConfig): Promise<SitemapObservation> {
  const observation: SitemapObservation = {
    shape: 'urlset',
    shardCount: 0,
    observedByFamily: emptyCounts(),
    observedPages: 0,
    observedOther: 0,
    locs: [],
    showDates: [],
    errors: [],
  }

  const entryUrl = `${config.target}${config.entryPath}`
  const entryXml = await request(entryUrl, config).then(r => r.text())
  observation.shape = detectShape(entryXml)

  if (observation.shape === 'urlset') {
    observation.locs = parseUrlset(entryXml)
    collectShowDates(observation.locs, observation.showDates)
    for (const loc of observation.locs) {
      countInto(observation, classifyLoc(loc), 1)
    }
    return observation
  }

  const shardUrls = parseSitemapIndex(entryXml).map(loc => rebaseOnTarget(loc, config.target))
  observation.shardCount = shardUrls.length

  const known = new Set<string>([PAGES_SHARD_ID, ...FAMILY_SHARD_IDS])
  const listed = new Set<string>()

  for (const shardUrl of shardUrls) {
    const id = shardIdFromUrl(shardUrl)
    listed.add(id)
    if (!known.has(id)) {
      observation.errors.push(`sitemap index lists unknown shard "${id}" (${shardUrl})`)
      continue
    }
    try {
      const xml = await request(shardUrl, config).then(r => r.text())
      // A shard must be a urlset. An index here would mean nested sharding
      // this monitor does not understand, and counting it as zero URLs would
      // read as a catastrophically empty family.
      const shape = detectShape(xml)
      if (shape !== 'urlset') {
        observation.errors.push(`shard "${id}" served a <${shape}> where a <urlset> was expected`)
        continue
      }
      const locs = parseUrlset(xml)
      // Not `push(...locs)`: the releases shard is already ~20k entries and
      // spreading it into the argument list approaches the engine's stack
      // argument limit.
      for (const loc of locs) observation.locs.push(loc)
      if (id === 'shows') collectShowDates(locs, observation.showDates)
      countInto(observation, id as Family | typeof PAGES_SHARD_ID, locs.length)
    } catch (error) {
      observation.errors.push(`shard "${id}": ${(error as Error).message}`)
    }
  }

  // Every family the sitemap claims to shard must actually be listed. A family
  // silently dropped from the index is thousands of URLs vanishing with no
  // other signal — the incident's exact shape.
  for (const family of FAMILY_SHARD_IDS) {
    if (!listed.has(family)) {
      observation.errors.push(`sitemap index is missing the "${family}" shard`)
    }
  }

  return observation
}

/** Slug counts per family, straight from the projection feed. */
export async function fetchExpectedCounts(
  config: MonitorConfig
): Promise<Record<Family, number>> {
  const url = `${config.apiBase}/sitemap/entries`
  const response = await fetch(url, {
    headers: { 'user-agent': USER_AGENT },
    signal: AbortSignal.timeout(config.fetchTimeoutMs),
  })
  if (!response.ok) {
    throw new Error(`GET ${url} returned ${response.status}`)
  }

  const body: unknown = await response.json()
  if (typeof body !== 'object' || body === null) {
    throw new Error(`GET ${url} did not return a JSON object`)
  }

  const record = body as Record<string, unknown>
  const counts = emptyCounts()
  for (const family of FAMILY_SHARD_IDS) {
    const rows = record[family]
    if (!Array.isArray(rows)) {
      // Coercing a missing family to 0 would report it as 100% drift and blame
      // the sitemap for an API-side problem. Fail on the real cause instead.
      throw new Error(`GET ${url} is missing the "${family}" family`)
    }
    // Count only rows the sitemap would actually emit: app/sitemap.ts drops
    // entries with an empty slug, so counting them here would manufacture
    // drift that does not exist.
    let emitted = 0
    for (const row of rows) {
      const slug = (row as { slug?: unknown } | null)?.slug
      if (typeof slug === 'string' && slug !== '') emitted++
    }
    counts[family] = emitted
  }
  return counts
}

/**
 * Probe sampled URLs, catching the inverse failure: a sitemap that is the right
 * size but advertises URLs that 404.
 *
 * Redirects are followed and the FINAL status is what counts. The sitemap
 * emits apex-host URLs (`https://psychichomily.com/...`) while the site is
 * served from `www`, so every entry legitimately redirects once; asserting on
 * the first hop would fail every sample.
 */
export async function sampleUrls(
  urls: readonly string[],
  config: MonitorConfig
): Promise<SampleResult[]> {
  return Promise.all(
    urls.map(async url => {
      try {
        const response = await fetch(url, {
          method: 'GET',
          headers: headers(config),
          redirect: 'follow',
          signal: AbortSignal.timeout(config.sampleTimeoutMs),
        })
        // Only the status matters; release the body rather than buffering a
        // full SSR page for each probe.
        await response.body?.cancel()
        return { url, status: response.status, ok: response.ok }
      } catch (error) {
        return { url, status: null, ok: false, error: (error as Error).message }
      }
    })
  )
}
