import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'
import { toTelemetryPath } from './rate-limit-telemetry'

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
  // A pure function, so it is imported statically: the `loadTelemetry()`
  // dance below exists only for the module-level samplers, and paying it here
  // would imply state where there is none.
  it.each([
    {
      input: '/shows?token=s3cret&q=user+typed+this',
      expected: '/shows',
      why: 'query strings carry feed tokens, magic-link tokens and search terms',
    },
    {
      input: '/shows#fragment',
      expected: '/shows',
      why: 'fragments go with the query',
    },
    {
      input: 'https://api.psychichomily.com/artists/1/graph',
      expected: '/artists/:id/graph',
      why: 'an absolute URL is reduced to its path',
    },
    {
      input: '//api.psychichomily.com/artists/1/graph',
      expected: '/artists/:id/graph',
      why: 'a protocol-relative URL loses its host too',
    },
    {
      input: '/artists/4821/releases',
      expected: '/artists/:id/releases',
      why: 'numeric ids',
    },
    {
      input: '/calendar/3f2504e0-4f89-11d3-9a0c-0305e82c3301',
      expected: '/calendar/:uuid',
      why: 'uuids',
    },
    {
      input: '/unsubscribe/AbCdEf0123456789AbCdEf0123',
      expected: '/unsubscribe/:opaque',
      why: 'a long non-slug segment is an identifier, not a name',
    },
    {
      input: '/feeds/phcal_SECRETFEEDTOKEN9/saved-shows.ics',
      expected: '/feeds/:token/saved-shows.ics',
      why: 'a credential prefix is scrubbed at ANY length, so a future short-token scheme cannot ride through on the length threshold',
    },
    {
      input: '/admin/phk_short',
      expected: '/admin/:token',
      why: 'the API-token prefix likewise',
    },
    {
      input: '/x/eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r',
      expected: '/x/:opaque',
      why: 'a JWT has dots and would have survived a denylist tuned for hex',
    },
    {
      input: '/x/v4uEcHqW-2Ls9Ez7tRk3Xb1PmQnAgYdF6CzJ',
      expected: '/x/:opaque',
      why: 'a base64url token has hyphens and would likewise have survived',
    },
    {
      input: '/artists/sunn-o/releases',
      expected: '/artists/sunn-o/releases',
      why: 'human-readable slugs are the entire point of the signal',
    },
    {
      input: '/collections/the-very-best-phoenix-doom-bands',
      expected: '/collections/the-very-best-phoenix-doom-bands',
      why: 'a LONG slug is still a slug, so length alone must not scrub it',
    },
    {
      input: undefined,
      expected: 'unknown',
      why: 'a missing endpoint',
    },
    {
      input: '',
      expected: 'unknown',
      why: 'an empty endpoint',
    },
    {
      input: 'https://api.psychichomily.com',
      expected: 'unknown',
      why: 'an origin with no path',
    },
  ])('scrubs $input to $expected ($why)', ({ input, expected }) => {
    expect(toTelemetryPath(input)).toBe(expected)
  })

  it('caps segment count and total length', () => {
    expect(toTelemetryPath('/a/b/c/d/e/f/g/h/i/j/k').endsWith('/...')).toBe(
      true
    )
    expect(toTelemetryPath(`/${'x-y'.repeat(200)}`).length).toBeLessThanOrEqual(
      123
    )
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

  it('keeps reporting after the clock jumps backwards', async () => {
    const { recordRateLimitHit } = await loadTelemetry()

    recordRateLimitHit({ endpoint: '/artists/1/releases' })
    expect(capturedMessages()).toHaveLength(1)

    // NTP correction, sleep/resume, VM migration. A naive elapsed-time
    // subtraction goes negative here, reads as "still cooling down", and
    // silently suppresses every report until wall time catches back up: the
    // one way this sampler could fail without a bound.
    vi.setSystemTime(Date.now() - 5 * 60_000)
    recordRateLimitHit({ endpoint: '/artists/2/releases' })

    expect(capturedMessages()).toHaveLength(2)
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
