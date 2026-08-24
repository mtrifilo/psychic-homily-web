import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'

/**
 * The cooldown samplers are module-level state, so every test re-imports the
 * module to get a fresh window. `@sentry/nextjs` is mocked globally in
 * test/setup.ts, and that mock instance survives resetModules, so the static
 * import above still observes the calls the re-imported module makes.
 */
async function loadTelemetry() {
  vi.resetModules()
  return import('./rate-limit-telemetry')
}

const capturedMessages = () => vi.mocked(Sentry.captureMessage).mock.calls
const breadcrumbs = () => vi.mocked(Sentry.addBreadcrumb).mock.calls

beforeEach(() => {
  vi.mocked(Sentry.captureMessage).mockClear()
  vi.mocked(Sentry.addBreadcrumb).mockClear()
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-08-23T12:00:00Z'))
})

afterEach(() => {
  vi.useRealTimers()
})

describe('toTelemetryPath', () => {
  it('drops the query string wholesale', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    // The query string is where feed tokens, magic-link tokens and user search
    // terms live, so it never survives.
    expect(toTelemetryPath('/shows?token=s3cret&q=user+typed+this')).toBe(
      '/shows'
    )
    expect(toTelemetryPath('/shows#fragment')).toBe('/shows')
  })

  it('reduces an absolute URL to its path', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    expect(toTelemetryPath('https://api.psychichomily.com/artists/1/graph')).toBe(
      '/artists/:id/graph'
    )
  })

  it('replaces numeric, uuid and opaque segments', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    expect(toTelemetryPath('/artists/4821/releases')).toBe(
      '/artists/:id/releases'
    )
    expect(
      toTelemetryPath('/calendar/3f2504e0-4f89-11d3-9a0c-0305e82c3301')
    ).toBe('/calendar/:uuid')
    expect(toTelemetryPath('/unsubscribe/AbCdEf0123456789AbCdEf0123')).toBe(
      '/unsubscribe/:opaque'
    )
  })

  it('scrubs a credential-prefixed segment at any length', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    // Real feed tokens are `phcal_` plus 64 hex characters, long enough for the
    // opaque-length rule to catch. The prefix rule is what keeps a SHORT token
    // from riding through on a threshold tuned for slugs.
    expect(toTelemetryPath('/feeds/phcal_SECRETFEEDTOKEN9/saved-shows.ics')).toBe(
      '/feeds/:token/saved-shows.ics'
    )
    expect(toTelemetryPath(`/feeds/phcal_${'a'.repeat(64)}/x.ics`)).toBe(
      '/feeds/:token/x.ics'
    )
    expect(toTelemetryPath('/admin/phk_shorttoken')).toBe('/admin/:token')
  })

  it('keeps human-readable slugs, which are the whole signal', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    expect(toTelemetryPath('/artists/sunn-o/releases')).toBe(
      '/artists/sunn-o/releases'
    )
    // A long but hyphenated segment is a slug, not a token.
    expect(toTelemetryPath('/collections/the-very-best-phoenix-doom-bands')).toBe(
      '/collections/the-very-best-phoenix-doom-bands'
    )
  })

  it('caps segment count and total length', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    const deep = toTelemetryPath('/a/b/c/d/e/f/g/h/i/j/k')
    expect(deep.endsWith('/...')).toBe(true)

    const long = toTelemetryPath(`/${'x-y'.repeat(200)}`)
    expect(long.length).toBeLessThanOrEqual(123)
  })

  it('falls back to a placeholder for an unusable endpoint', async () => {
    const { toTelemetryPath } = await loadTelemetry()
    expect(toTelemetryPath(undefined)).toBe('unknown')
    expect(toTelemetryPath('')).toBe('unknown')
    expect(toTelemetryPath('https://api.psychichomily.com')).toBe('unknown')
  })
})

describe('recordRateLimitHit', () => {
  it('breadcrumbs every 429 and promotes the first to a warning event', async () => {
    const { recordRateLimitHit } = await loadTelemetry()

    recordRateLimitHit({
      endpoint: '/artists/12/releases?limit=50',
      retryAfter: 60,
      requestId: 'req-1',
    })

    expect(breadcrumbs()).toHaveLength(1)
    expect(breadcrumbs()[0][0]).toMatchObject({
      category: 'rate-limit',
      level: 'warning',
      message: '429 /artists/:id/releases',
    })

    expect(capturedMessages()).toHaveLength(1)
    const [message, context] = capturedMessages()[0]
    expect(message).toBe('Client rate limited (HTTP 429)')
    expect(context).toMatchObject({
      level: 'warning',
      tags: { error_type: 'rate_limited', status: 429, has_retry_after: true },
      extra: {
        endpoint: '/artists/:id/releases',
        retryAfterSeconds: 60,
        requestId: 'req-1',
        suppressedSinceLastReport: 0,
      },
    })
  })

  it('never ships a query string or a full URL', async () => {
    const { recordRateLimitHit } = await loadTelemetry()

    recordRateLimitHit({
      endpoint: 'https://api.psychichomily.com/shows?token=s3cret&q=slayer',
    })

    const serialized = JSON.stringify([breadcrumbs(), capturedMessages()])
    expect(serialized).not.toContain('s3cret')
    expect(serialized).not.toContain('slayer')
    expect(serialized).not.toContain('api.psychichomily.com')
  })

  it('folds a burst into one event and reports the suppressed count', async () => {
    const { recordRateLimitHit, RATE_LIMIT_REPORT_COOLDOWN_MS } =
      await loadTelemetry()

    // One entity page fans out into many parallel reads, so an exhausted
    // budget arrives as a burst inside a few hundred milliseconds.
    for (let i = 0; i < 12; i++) {
      recordRateLimitHit({ endpoint: `/artists/${i}/releases` })
    }

    expect(breadcrumbs()).toHaveLength(12)
    expect(capturedMessages()).toHaveLength(1)

    // Past the cooldown, the next 429 reports again and carries the volume
    // that was folded in, so deduplication does not lose the count.
    vi.setSystemTime(Date.now() + RATE_LIMIT_REPORT_COOLDOWN_MS + 1)
    recordRateLimitHit({ endpoint: '/artists/99/releases' })

    expect(capturedMessages()).toHaveLength(2)
    expect(capturedMessages()[1][1]).toMatchObject({
      extra: { suppressedSinceLastReport: 11 },
    })
  })

  it('flags a missing Retry-After, the production case', async () => {
    const { recordRateLimitHit } = await loadTelemetry()

    recordRateLimitHit({ endpoint: '/artists/12/releases' })

    expect(capturedMessages()[0][1]).toMatchObject({
      tags: { has_retry_after: false },
    })
  })
})

describe('reportRateLimitExhausted', () => {
  it('reports at error level with the query family and attempt count', async () => {
    const { reportRateLimitExhausted } = await loadTelemetry()

    reportRateLimitExhausted({
      queryFamily: 'artists/releases',
      attempts: 4,
      requestId: 'req-9',
    })

    expect(capturedMessages()).toHaveLength(1)
    const [message, context] = capturedMessages()[0]
    expect(message).toBe('Rate limit retries exhausted (HTTP 429)')
    expect(context).toMatchObject({
      level: 'error',
      tags: { error_type: 'rate_limit_exhausted', status: 429 },
      extra: {
        queryFamily: 'artists/releases',
        attempts: 4,
        requestId: 'req-9',
      },
    })
    // No endpoint is available from the query cache, so none is invented.
    expect(
      (context as { extra: Record<string, unknown> }).extra.endpoint
    ).toBeUndefined()
  })

  it('samples on a cooldown independent of the hit signal', async () => {
    const { recordRateLimitHit, reportRateLimitExhausted } =
      await loadTelemetry()

    // A hit having just claimed its slot must not suppress the exhaustion
    // event: the two signals answer different questions.
    recordRateLimitHit({ endpoint: '/artists/1/releases' })
    reportRateLimitExhausted({ queryFamily: 'artists/releases', attempts: 4 })

    expect(capturedMessages()).toHaveLength(2)

    // A second exhaustion inside the window folds in.
    reportRateLimitExhausted({ queryFamily: 'artists/graph', attempts: 4 })
    expect(capturedMessages()).toHaveLength(2)
  })
})
