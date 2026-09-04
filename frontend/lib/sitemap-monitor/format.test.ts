import { describe, expect, it } from 'vitest'
import { SITEMAP_FAMILIES, type Family } from '@/app/sitemap-shards'
import { resolveConfig } from './config'
import { evaluate, type EvaluationInput } from './evaluate'
import {
  formatConsoleReport,
  formatCrashPayload,
  formatDiscordPayload,
  truncate,
} from './format'

const config = resolveConfig({})
const NOW = new Date('2026-07-30T12:00:00Z')

function counts(overrides: Partial<Record<Family, number>> = {}): Record<Family, number> {
  const base = Object.fromEntries(SITEMAP_FAMILIES.map(f => [f, 0])) as Record<Family, number>
  return { ...base, ...overrides }
}

function report(overrides: Partial<EvaluationInput> = {}) {
  return evaluate(
    {
      shape: 'index',
      shardCount: 10,
      observedByFamily: counts({ shows: 1458 }),
      observedPages: 12,
      observedOther: 0,
      expectedByFamily: counts({ shows: 1458 }),
      observedByShard: new Map(),
      expectedByShard: new Map(),
      unservedShards: [],
      futureShowCount: 500,
      samples: [{ url: 'https://psychichomily.com/shows/a', status: 200, ok: true }],
      errors: [],
      ...overrides,
    },
    config
  )
}

describe('truncate', () => {
  it('leaves a short string alone', () => {
    expect(truncate('abc', 10)).toBe('abc')
  })

  it('caps an over-long string at the limit', () => {
    const result = truncate('x'.repeat(50), 10)
    expect(result).toHaveLength(10)
    expect(result.endsWith('…')).toBe(true)
  })
})

describe('formatConsoleReport', () => {
  it('states the verdict, the target and the shape', () => {
    const output = formatConsoleReport(report())
    expect(output).toContain('Sitemap freshness — PASS')
    expect(output).toContain('https://psychichomily.com')
    expect(output).toContain('<index> (10 shards)')
  })

  it('shows the numbers for a drifting family', () => {
    const output = formatConsoleReport(
      report({ observedByFamily: counts({ shows: 114 }), expectedByFamily: counts({ shows: 3498 }) })
    )
    expect(output).toContain('Sitemap freshness — FAIL')
    expect(output).toContain('sitemap 114 / api 3498')
  })

  it('lists unreachable sampled URLs', () => {
    const output = formatConsoleReport(
      report({
        samples: [{ url: 'https://psychichomily.com/shows/gone', status: 404, ok: false }],
      })
    )
    expect(output).toContain('unreachable:')
    expect(output).toContain('https://psychichomily.com/shows/gone → 404')
  })
})

describe('formatDiscordPayload', () => {
  // A bare "check failed" gets muted. The drift numbers have to be in the
  // message body itself.
  it('puts the drift numbers in the alert body', () => {
    const payload = formatDiscordPayload(
      report({
        observedByFamily: counts({ shows: 114 }),
        expectedByFamily: counts({ shows: 3498 }),
        futureShowCount: 0,
      }),
      NOW
    )
    const embed = payload.embeds[0]
    expect(embed.title).toBe('Sitemap freshness check FAILED')
    expect(embed.description).toContain('sitemap 114 / api 3498')
    expect(embed.description).toContain('Upcoming shows: **0**')
    expect(embed.fields?.[0].value).toContain('shows: sitemap 114 vs API 3498')
  })

  it('marks a passing run green with no failure field', () => {
    const payload = formatDiscordPayload(report(), NOW)
    expect(payload.embeds[0].title).toBe('Sitemap freshness check passed')
    expect(payload.embeds[0].color).toBe(0x00ff00)
    expect(payload.embeds[0].fields).toBeUndefined()
  })

  it('stamps the run time', () => {
    expect(formatDiscordPayload(report(), NOW).embeds[0].timestamp).toBe(
      '2026-07-30T12:00:00.000Z'
    )
  })

  it('reports the shard count only for an index', () => {
    const single = formatDiscordPayload(report({ shape: 'urlset', shardCount: 0 }), NOW)
    expect(single.embeds[0].description).toContain('`<urlset>`')
    expect(single.embeds[0].description).not.toContain('shards')
  })

  /**
   * Discord rejects an oversized embed with a 400, which would lose the alert
   * at exactly the moment it matters. Every string must be capped.
   */
  it('stays within Discord embed limits when everything fails at once', () => {
    const payload = formatDiscordPayload(
      report({
        samples: Array.from({ length: 200 }, (_, i) => ({
          url: `https://psychichomily.com/shows/${'x'.repeat(80)}-${i}`,
          status: 404,
          ok: false,
        })),
      }),
      NOW
    )
    const embed = payload.embeds[0]
    expect(embed.title.length).toBeLessThanOrEqual(256)
    expect(embed.description?.length ?? 0).toBeLessThanOrEqual(4096)
    for (const field of embed.fields ?? []) {
      expect(field.value.length).toBeLessThanOrEqual(1024)
    }
    expect(embed.fields?.length ?? 0).toBeLessThanOrEqual(25)
  })
})

/**
 * "The monitor crashed" and "the sitemap is fine" must never look the same from
 * the outside, so the unrunnable case gets its own alert rather than silence.
 */
describe('formatCrashPayload', () => {
  it('names the target and carries the cause', () => {
    const embed = formatCrashPayload(
      'https://psychichomily.com',
      'GET https://api.psychichomily.com/sitemap/entries returned 404',
      NOW
    ).embeds[0]
    expect(embed.title).toBe('Sitemap freshness check could not run')
    expect(embed.description).toContain('https://psychichomily.com')
    expect(embed.description).toContain('returned 404')
    expect(embed.color).toBe(0xffa500)
  })

  // The limit is owned by format.ts alone; an oversized embed 400s and the
  // alert is lost at exactly the moment it matters.
  it('keeps an enormous error message within the description limit', () => {
    const embed = formatCrashPayload('https://psychichomily.com', 'x'.repeat(20_000), NOW)
      .embeds[0]
    expect(embed.description?.length ?? 0).toBeLessThanOrEqual(4096)
  })
})
