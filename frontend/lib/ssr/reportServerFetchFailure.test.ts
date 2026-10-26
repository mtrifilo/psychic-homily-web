import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
}))

import * as Sentry from '@sentry/nextjs'
import {
  listPathFamily,
  listQueryParamNames,
  REPORT_WINDOW_MS,
  reportServerFetchFailure,
  resetServerFetchReportThrottleForTests,
} from './reportServerFetchFailure'

// Each test starts with every route's report window open.
beforeEach(() => {
  resetServerFetchReportThrottleForTests()
})

function response(status: number, headers: Record<string, string> = {}): Response {
  return new Response(null, { status, headers })
}

describe('reportServerFetchFailure', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reports a 429 with its path family and Retry-After', () => {
    reportServerFetchFailure({
      response: response(429, { 'Retry-After': '60' }),
      service: 'artist-page',
      pathFamily: '/artists/{slug}',
      notFoundIsAnswer: true,
      context: { slug: 'some-artist' },
    })

    expect(Sentry.captureMessage).toHaveBeenCalledWith('artist-page: API returned 429', {
      level: 'error',
      tags: { service: 'artist-page', path_family: '/artists/{slug}', status: '429' },
      extra: {
        slug: 'some-artist',
        pathFamily: '/artists/{slug}',
        status: 429,
        retryAfter: '60',
        suppressedSinceLastReport: 0,
      },
    })
  })

  it('leaves Retry-After out when the backend sent none', () => {
    reportServerFetchFailure({
      response: response(503),
      service: 'show-page',
      pathFamily: '/shows/{slug}',
      notFoundIsAnswer: true,
    })

    expect(Sentry.captureMessage).toHaveBeenCalledWith('show-page: API returned 503', {
      level: 'error',
      tags: { service: 'show-page', path_family: '/shows/{slug}', status: '503' },
      extra: { pathFamily: '/shows/{slug}', status: 503, suppressedSinceLastReport: 0 },
    })
  })

  it.each([400, 401, 403, 422, 429])('reports a %d', status => {
    reportServerFetchFailure({
      response: response(status),
      service: 'tag-page',
      pathFamily: '/tags/{slug}/detail',
      notFoundIsAnswer: true,
    })
    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
  })

  it.each([500, 502, 503, 504])('reports a %d', status => {
    reportServerFetchFailure({
      response: response(status),
      service: 'tag-page',
      pathFamily: '/tags/{slug}/detail',
      notFoundIsAnswer: true,
    })
    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
  })

  it('stays silent on a 404 that is an answer', () => {
    reportServerFetchFailure({
      response: response(404),
      service: 'artist-page',
      pathFamily: '/artists/{slug}',
      notFoundIsAnswer: true,
    })
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it('reports a 404 that is not an answer', () => {
    reportServerFetchFailure({
      response: response(404),
      service: 'shows-list',
      pathFamily: '/shows/upcoming',
      notFoundIsAnswer: false,
    })
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'shows-list: API returned 404',
      expect.objectContaining({ extra: { pathFamily: '/shows/upcoming', status: 404, suppressedSinceLastReport: 0 } })
    )
  })

  it('passes the level through', () => {
    reportServerFetchFailure({
      response: response(429),
      service: 'show-page',
      pathFamily: '/shows/{slug}/timeline',
      notFoundIsAnswer: true,
      level: 'warning',
    })
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'show-page: API returned 429',
      expect.objectContaining({ level: 'warning' })
    )
  })
})

describe('listPathFamily', () => {
  it('drops the origin and the query string', () => {
    expect(listPathFamily('https://api.example.com/shows/upcoming?limit=50&cursor=abc')).toBe(
      '/shows/upcoming'
    )
  })

  it('accepts a relative URL', () => {
    expect(listPathFamily('/api/venues/cities?x=1')).toBe('/api/venues/cities')
  })
})

describe('listQueryParamNames', () => {
  it('names the parameters without their values', () => {
    expect(listQueryParamNames('https://api.example.com/venues?limit=200&cursor=abc')).toBe(
      'limit,cursor'
    )
  })

  it('is empty for a URL with no query', () => {
    expect(listQueryParamNames('https://api.example.com/venues')).toBe('')
  })
})

describe('reportServerFetchFailure throttle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-26T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function fail(status: number, pathFamily = '/artists/{slug}') {
    reportServerFetchFailure({
      response: response(status),
      service: 'artist-page',
      pathFamily,
      notFoundIsAnswer: true,
    })
  }

  function suppressedCounts(): unknown[] {
    return vi
      .mocked(Sentry.captureMessage)
      .mock.calls.map(([, hint]) => (hint as { extra: Record<string, unknown> }).extra.suppressedSinceLastReport)
  }

  it('reports the first refusal of a route, with nothing held back', () => {
    fail(429)

    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
    expect(suppressedCounts()).toEqual([0])
  })

  it('holds back repeats inside the window and counts them on the next event', () => {
    fail(429)
    fail(429)
    fail(503)
    fail(429)
    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(REPORT_WINDOW_MS)
    fail(429)

    expect(Sentry.captureMessage).toHaveBeenCalledTimes(2)
    expect(suppressedCounts()).toEqual([0, 3])
  })

  it('throttles 5xx on the same window as 429', () => {
    fail(500)
    fail(502)
    fail(429)

    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'artist-page: API returned 500',
      expect.anything()
    )
  })

  it('keeps a separate window per route template', () => {
    fail(429, '/artists/{slug}')
    fail(429, '/shows/{slug}')
    fail(429, '/artists/{slug}')

    expect(Sentry.captureMessage).toHaveBeenCalledTimes(2)
  })

  it('does not count an answering 404 as a held-back failure', () => {
    fail(429)
    fail(404)
    vi.advanceTimersByTime(REPORT_WINDOW_MS)
    fail(429)

    expect(suppressedCounts()).toEqual([0, 0])
  })
})
