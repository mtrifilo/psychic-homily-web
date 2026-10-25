import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
}))

import * as Sentry from '@sentry/nextjs'
import { listPathFamily, reportServerFetchFailure } from './reportServerFetchFailure'

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
      extra: { pathFamily: '/shows/{slug}', status: 503 },
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
      expect.objectContaining({ extra: { pathFamily: '/shows/upcoming', status: 404 } })
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
