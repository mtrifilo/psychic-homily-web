/**
 * Tests for the orchestration that decides whether anyone HEARS about a
 * failure.
 *
 * This is the highest-stakes logic in the module and the least obvious: an
 * inverted condition on the notify gate would ship green and be
 * indistinguishable from a healthy system — the exact trap the monitor exists
 * to close. Formatting is covered in format.test.ts; what is asserted here is
 * whether a POST happens at all, and what the process exits with.
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ENTITY_SHARD_IDS,
  PAGES_SHARD_ID,
  SHOW_SHARD_IDS,
  SITEMAP_FAMILIES,
} from '@/app/sitemap-shards'
import type { Env } from './config'
import { main } from './cli'

const WEBHOOK = 'https://discord.example/api/webhooks/1/secret-token'
const TARGET = 'https://sitemap-monitor.test'
const API = 'https://api.sitemap-monitor.test'

const ALL_IDS = [PAGES_SHARD_ID, ...ENTITY_SHARD_IDS]

/** The shows sub-shard the stubbed sitemap serves show URLs from. */
const [SHOW_SHARD] = SHOW_SHARD_IDS

function urlset(locs: string[]): string {
  return `<urlset>${locs.map(l => `<url><loc>${l}</loc></url>`).join('')}</urlset>`
}

function index(): string {
  return `<sitemapindex>${ALL_IDS.map(
    id => `<sitemap><loc>${TARGET}/sitemap/${id}.xml</loc></sitemap>`
  ).join('')}</sitemapindex>`
}

/** A show far enough ahead that the freshness floor is satisfied. */
const FUTURE_SHOWS = Array.from({ length: 20 }, (_, i) => `${TARGET}/shows/2099-01-${i + 1}-a`)

interface WorldOptions {
  /** Shows served by the sitemap; defaults to a healthy future-dated set. */
  shows?: string[]
  /** Shows the API claims to hold. */
  apiShows?: number
  /** Status returned by the Discord webhook. */
  webhookStatus?: number
  /** Make the API feed unreachable. */
  apiDown?: boolean
}

/**
 * Stub the whole network: the API feed, the sitemap index, every shard, the
 * sampled page probes, and the Discord webhook. Returns the spy so tests can
 * assert on what was POSTed.
 */
function stubWorld(options: WorldOptions = {}) {
  const {
    shows = FUTURE_SHOWS,
    apiShows = FUTURE_SHOWS.length,
    webhookStatus = 204,
    apiDown = false,
  } = options

  const fetchSpy = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.startsWith(WEBHOOK)) {
      // `null` body, not '': constructing a Response with a body and a
      // null-body status like 204 throws, which would look like a delivery
      // failure rather than the success Discord actually returns.
      return new Response(null, { status: webhookStatus })
    }
    if (url.startsWith(`${API}/sitemap/entries`)) {
      if (apiDown) return new Response('down', { status: 503 })
      // The unsharded feed: keyed by FAMILY, not by shard id.
      const body = Object.fromEntries(
        SITEMAP_FAMILIES.map(f => [
          f,
          f === 'shows' ? Array.from({ length: apiShows }, (_, i) => ({ slug: `s${i}` })) : [],
        ])
      )
      return new Response(JSON.stringify(body))
    }
    if (url === `${TARGET}/sitemap-index`) {
      return new Response(index(), { headers: { 'content-type': 'application/xml' } })
    }
    if (url.startsWith(`${TARGET}/sitemap/`)) {
      const isShows = url.endsWith(`/${SHOW_SHARD}.xml`)
      return new Response(urlset(isShows ? shows : []), {
        headers: { 'content-type': 'application/xml' },
      })
    }
    // Sampled page probes.
    void init
    return new Response('ok')
  })

  vi.stubGlobal('fetch', fetchSpy)
  return fetchSpy
}

function env(overrides: Record<string, string> = {}): Env {
  return {
    SITEMAP_MONITOR_TARGET: TARGET,
    SITEMAP_MONITOR_API_BASE: API,
    SITEMAP_MONITOR_RETRY_DELAY_MS: '0',
    ...overrides,
  }
}

const NOW = new Date('2026-07-31T12:00:00Z')

function webhookPosts(spy: ReturnType<typeof stubWorld>) {
  return spy.mock.calls.filter(([url]) => String(url).startsWith(WEBHOOK))
}

function postedEmbed(spy: ReturnType<typeof stubWorld>) {
  const [, init] = webhookPosts(spy)[0]
  return JSON.parse(String((init as RequestInit).body)).embeds[0]
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('main', () => {
  it('exits 0 and stays silent when the sitemap is healthy', async () => {
    const spy = stubWorld()
    const code = await main(env({ DISCORD_WEBHOOK_URL: WEBHOOK }), NOW)

    expect(code).toBe(0)
    expect(webhookPosts(spy)).toHaveLength(0)
  })

  it('exits 1 and posts the failure when the sitemap is stale', async () => {
    // Sitemap serves 2 shows against an API holding 400 — drift plus a
    // freshness shortfall.
    const spy = stubWorld({ shows: FUTURE_SHOWS.slice(0, 2), apiShows: 400 })
    const code = await main(env({ DISCORD_WEBHOOK_URL: WEBHOOK }), NOW)

    expect(code).toBe(1)
    expect(webhookPosts(spy)).toHaveLength(1)
    expect(postedEmbed(spy).title).toBe('Sitemap freshness check FAILED')
  })

  it('posts a green embed on success only when asked to', async () => {
    const spy = stubWorld()
    const code = await main(
      env({ DISCORD_WEBHOOK_URL: WEBHOOK, SITEMAP_MONITOR_NOTIFY_ON_SUCCESS: 'true' }),
      NOW
    )

    expect(code).toBe(0)
    expect(webhookPosts(spy)).toHaveLength(1)
    expect(postedEmbed(spy).title).toBe('Sitemap freshness check passed')
  })

  it('runs the check even with no webhook configured', async () => {
    const spy = stubWorld({ shows: FUTURE_SHOWS.slice(0, 2), apiShows: 400 })
    expect(await main(env(), NOW)).toBe(1)
    expect(webhookPosts(spy)).toHaveLength(0)
  })

  // "The monitor crashed" and "the sitemap is fine" must never look the same.
  it('posts the orange crash embed and exits 2 when the API feed is down', async () => {
    const spy = stubWorld({ apiDown: true })
    const code = await main(env({ DISCORD_WEBHOOK_URL: WEBHOOK }), NOW)

    expect(code).toBe(2)
    const embed = postedEmbed(spy)
    expect(embed.title).toBe('Sitemap freshness check could not run')
    expect(embed.color).toBe(0xffa500)
    expect(embed.description).toContain('503')
  })

  /**
   * A threshold typo'd in the GitHub UI must not kill the monitor silently.
   * Without this the check would be dead every day with nothing but a red
   * scheduled run — which GitHub eventually disables on its own.
   */
  it('alerts on a malformed threshold rather than dying quietly', async () => {
    const spy = stubWorld()
    const code = await main(
      env({ DISCORD_WEBHOOK_URL: WEBHOOK, SITEMAP_MONITOR_DRIFT_RATIO: '0.2x' }),
      NOW
    )

    expect(code).toBe(2)
    expect(webhookPosts(spy)).toHaveLength(1)
    const embed = postedEmbed(spy)
    expect(embed.title).toBe('Sitemap freshness check could not run')
    expect(embed.description).toContain('SITEMAP_MONITOR_DRIFT_RATIO')
  })

  it('exits 2 when a healthy run cannot deliver its notification', async () => {
    stubWorld({ webhookStatus: 500 })
    const code = await main(
      env({ DISCORD_WEBHOOK_URL: WEBHOOK, SITEMAP_MONITOR_NOTIFY_ON_SUCCESS: 'true' }),
      NOW
    )
    expect(code).toBe(2)
  })

  /**
   * A failed delivery must not overwrite a real sitemap failure with the
   * "monitor broken" code — the stale sitemap is still the finding.
   */
  it('keeps exit 1 when the sitemap failed AND the webhook post failed', async () => {
    stubWorld({ shows: FUTURE_SHOWS.slice(0, 2), apiShows: 400, webhookStatus: 500 })
    expect(await main(env({ DISCORD_WEBHOOK_URL: WEBHOOK }), NOW)).toBe(1)
  })

  /**
   * The webhook path carries a bearer-equivalent secret. A malformed
   * DISCORD_WEBHOOK_URL makes fetch throw "Failed to parse URL from <the-url>",
   * which would print the token straight into a public Actions log.
   *
   * The webhook call must THROW here — with a 2xx stub the catch block never
   * runs and this test would pass with the scrub deleted.
   */
  it('scrubs the webhook URL out of a failed-post error', async () => {
    const spy = stubWorld()
    spy.mockImplementation(async (url: string) => {
      if (url.startsWith(WEBHOOK)) {
        throw new Error(`Failed to parse URL from ${WEBHOOK}`)
      }
      throw new Error('unreachable in this test')
    })

    const errors: string[] = []
    vi.spyOn(console, 'error').mockImplementation(msg => errors.push(String(msg)))

    await main(
      env({ DISCORD_WEBHOOK_URL: WEBHOOK, SITEMAP_MONITOR_DRIFT_RATIO: 'nonsense' }),
      NOW
    )

    const scrubbed = errors.filter(line => line.includes('[redacted]'))
    expect(scrubbed.length).toBeGreaterThan(0)
    for (const line of errors) {
      expect(line).not.toContain('secret-token')
    }
  })
})
