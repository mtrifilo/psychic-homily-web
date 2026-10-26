import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

import * as Sentry from '@sentry/nextjs'
import { getVenue } from './archiveApi'
import { resetServerFetchReportThrottleForTests } from '@/lib/ssr/reportServerFetchFailure'

// Each test starts with every route's report window open.
beforeEach(() => {
  resetServerFetchReportThrottleForTests()
})

const fetchMock = vi.fn()

describe('getVenue failure reporting', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reports a 429 and answers unavailable, not missing', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(null, { status: 429, headers: { 'Retry-After': '60' } })
    )

    await expect(getVenue('rate-limited-venue')).resolves.toEqual({ status: 'unavailable' })
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'venue-page: API returned 429',
      expect.objectContaining({
        tags: { service: 'venue-page', path_family: '/venues/{slug}', status: '429' },
        extra: expect.objectContaining({ idOrSlug: 'rate-limited-venue', retryAfter: '60' }),
      })
    )
  })

  it('answers missing and stays silent on a 404', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 404 }))

    await expect(getVenue('no-such-venue')).resolves.toEqual({ status: 'missing' })
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it('reports a 5xx', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 503 }))

    await expect(getVenue('broken-venue')).resolves.toEqual({ status: 'unavailable' })
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'venue-page: API returned 503',
      expect.anything()
    )
  })
})
