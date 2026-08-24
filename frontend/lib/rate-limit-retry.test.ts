import { describe, it, expect } from 'vitest'
import {
  RATE_LIMIT_FALLBACK_BASE_MS,
  RATE_LIMIT_JITTER_RATIO,
  RATE_LIMIT_MAX_BASE_DELAY_MS,
  RATE_LIMIT_MAX_RETRIES,
  isRateLimitError,
  queryRetryDelay,
  rateLimitRetryDelay,
  shouldRetryQuery,
} from './rate-limit-retry'

const httpError = (status: number, retryAfter?: number) =>
  Object.assign(new Error(`HTTP ${status}`), { status, retryAfter })

/** Deterministic jitter sources, so every delay assertion is exact. */
const noJitter = () => 0
const maxJitter = () => 1

describe('isRateLimitError', () => {
  it('recognizes a 429 and nothing else', () => {
    expect(isRateLimitError(httpError(429))).toBe(true)
    expect(isRateLimitError(httpError(403))).toBe(false)
    expect(isRateLimitError(httpError(500))).toBe(false)
    expect(isRateLimitError(new Error('network'))).toBe(false)
    expect(isRateLimitError(null)).toBe(false)
    expect(isRateLimitError(undefined)).toBe(false)
  })
})

describe('shouldRetryQuery', () => {
  it('retries a 429 up to the rate-limit budget in the browser', () => {
    const error = httpError(429)
    for (let attempt = 0; attempt < RATE_LIMIT_MAX_RETRIES; attempt++) {
      expect(shouldRetryQuery(attempt, error, true)).toBe(true)
    }
    expect(shouldRetryQuery(RATE_LIMIT_MAX_RETRIES, error, true)).toBe(false)
  })

  it('leaves a 429 terminal on the server so a render cannot stall', () => {
    expect(shouldRetryQuery(0, httpError(429), false)).toBe(false)
  })

  it('defaults to the browser branch under jsdom, where window exists', () => {
    // Guards the runtime-derived default: React Query calls this with two
    // arguments, so a broken default would silently disable the whole fix.
    expect(shouldRetryQuery(0, httpError(429))).toBe(true)
  })

  // The behaviour this ticket must not regress: every other 4xx stays terminal.
  it.each([400, 401, 403, 404, 409, 422])(
    'never retries a %i',
    status => {
      expect(shouldRetryQuery(0, httpError(status), true)).toBe(false)
      expect(shouldRetryQuery(0, httpError(status), false)).toBe(false)
    }
  )

  it('keeps the three-retry budget for 5xx', () => {
    const error = httpError(500)
    expect(shouldRetryQuery(0, error, true)).toBe(true)
    expect(shouldRetryQuery(1, error, true)).toBe(true)
    expect(shouldRetryQuery(2, error, true)).toBe(true)
    expect(shouldRetryQuery(3, error, true)).toBe(false)
  })

  it('keeps the three-retry budget for status-less network errors', () => {
    const error = new Error('Network error')
    expect(shouldRetryQuery(0, error, true)).toBe(true)
    expect(shouldRetryQuery(2, error, true)).toBe(true)
    expect(shouldRetryQuery(3, error, true)).toBe(false)
  })

  it('retries a 5xx on the server, unlike a 429', () => {
    expect(shouldRetryQuery(0, httpError(500), false)).toBe(true)
  })
})

describe('rateLimitRetryDelay', () => {
  it('follows Retry-After when the header was readable', () => {
    // 5s is under the ceiling, so it is honored as given.
    expect(rateLimitRetryDelay(0, httpError(429, 5), noJitter)).toBe(5_000)
  })

  it('clamps the backend constant Retry-After to the per-attempt ceiling', () => {
    // The backend always says 60, which is a whole limiter window rather than a
    // computed reset. Obeying it literally would park a block on a spinner for
    // a minute.
    expect(rateLimitRetryDelay(0, httpError(429, 60), noJitter)).toBe(
      RATE_LIMIT_MAX_BASE_DELAY_MS
    )
  })

  it('still applies jitter at the ceiling', () => {
    // Regression guard. Clamping AFTER jitter instead would collapse every
    // saturated delay onto the same number and retry the whole page in
    // lockstep, recreating the burst that exhausted the budget.
    const jittered = rateLimitRetryDelay(0, httpError(429, 60), maxJitter)
    expect(jittered).toBeGreaterThan(RATE_LIMIT_MAX_BASE_DELAY_MS)
    expect(jittered).toBe(
      RATE_LIMIT_MAX_BASE_DELAY_MS * (1 + RATE_LIMIT_JITTER_RATIO)
    )
  })

  it('backs off exponentially when Retry-After is absent', () => {
    // The production path: CORS hides the header from the browser.
    expect(rateLimitRetryDelay(0, httpError(429), noJitter)).toBe(
      RATE_LIMIT_FALLBACK_BASE_MS
    )
    expect(rateLimitRetryDelay(1, httpError(429), noJitter)).toBe(
      RATE_LIMIT_FALLBACK_BASE_MS * 2
    )
    expect(rateLimitRetryDelay(2, httpError(429), noJitter)).toBe(
      RATE_LIMIT_FALLBACK_BASE_MS * 4
    )
  })

  it('ignores an unusable Retry-After and falls back to backoff', () => {
    for (const bad of [0, -5, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(rateLimitRetryDelay(0, httpError(429, bad), noJitter)).toBe(
        RATE_LIMIT_FALLBACK_BASE_MS
      )
    }
  })

  it('keeps the total wait inside one limiter window', () => {
    // The guarantee the constants are chosen for: worst case across the whole
    // budget, with the constant Retry-After and maximum jitter on every
    // attempt, is exactly one 60s window.
    const worstCase = Array.from(
      { length: RATE_LIMIT_MAX_RETRIES },
      (_, attempt) =>
        rateLimitRetryDelay(attempt, httpError(429, 60), maxJitter)
    ).reduce((total, delay) => total + delay, 0)

    expect(worstCase).toBeLessThanOrEqual(60_000)
  })
})

describe('queryRetryDelay', () => {
  it('routes 429s through the rate-limit schedule', () => {
    expect(queryRetryDelay(0, httpError(429, 5), noJitter)).toBe(5_000)
  })

  it("leaves non-429 retry timing on React Query's own curve", () => {
    expect(queryRetryDelay(0, httpError(500))).toBe(1_000)
    expect(queryRetryDelay(1, httpError(500))).toBe(2_000)
    expect(queryRetryDelay(2, httpError(500))).toBe(4_000)
    // Capped at 30s, matching the library default this replaces.
    expect(queryRetryDelay(20, httpError(500))).toBe(30_000)
    expect(queryRetryDelay(0, new Error('network'))).toBe(1_000)
  })
})
