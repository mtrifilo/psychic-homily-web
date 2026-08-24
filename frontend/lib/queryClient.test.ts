import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import * as Sentry from '@sentry/nextjs'

function createDeferred<T>() {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>(res => {
    resolve = res
  })
  return { promise, resolve }
}

describe('queryClient module', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('getQueryClient', () => {
    it('returns a QueryClient instance', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      expect(client).toBeInstanceOf(QueryClient)
    })

    it('returns the same client on subsequent calls in browser', async () => {
      const { getQueryClient } = await import('./queryClient')

      const client1 = getQueryClient()
      const client2 = getQueryClient()

      expect(client1).toBe(client2)
    })

    it('configures default stale time', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const defaults = client.getDefaultOptions()
      expect(defaults.queries?.staleTime).toBe(15 * 60 * 1000) // 15 minutes
    })

    it('configures default gc time', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const defaults = client.getDefaultOptions()
      expect(defaults.queries?.gcTime).toBe(30 * 60 * 1000) // 30 minutes
    })

    it('configures mutation retry to 0', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const defaults = client.getDefaultOptions()
      expect(defaults.mutations?.retry).toBe(0)
    })
  })

  describe('queryKeys', () => {
    it('isolates authenticated follow and show-save state by user', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.follows.entity('artists', 7, 42)).not.toEqual(
        queryKeys.follows.entity('artists', 7, 84)
      )
      expect(queryKeys.follows.batch('artists', [7], 42)).not.toEqual(
        queryKeys.follows.batch('artists', [7], 84)
      )
      expect(queryKeys.savedShows.count(9, true, 42)).not.toEqual(
        queryKeys.savedShows.count(9, true, 84)
      )
      expect(queryKeys.savedShows.countBatch([9], true, 42)).not.toEqual(
        queryKeys.savedShows.countBatch([9], true, 84)
      )
    })

    it('normalizes venue search queries to lowercase', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.venues.search('REBEL LOUNGE')).toEqual([
        'venues',
        'search',
        'rebel lounge',
      ])
    })

    it('normalizes artist search queries to lowercase', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.artists.search('THE BAND')).toEqual([
        'artists',
        'search',
        'the band',
      ])
    })
  })

  describe('createInvalidateQueries', () => {
    it('creates invalidate helpers for auth', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.auth()

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['auth'],
      })
    })

    it('creates invalidate helpers for shows', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.shows()

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['shows'],
      })
    })

    it('creates invalidate helpers for specific show', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.show('show-789')

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['shows', 'detail', 'show-789'],
      })
    })

    it('creates invalidate helpers for artists', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.artists()

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['artists'],
      })
    })

    it('creates invalidate helpers for venues', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.venues()

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['venues'],
      })
    })

    it('creates invalidate helpers for savedShows', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.savedShows()

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['savedShows'],
      })
    })

    it('creates an invalidate helper for personal charts', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        cancelQueries: vi.fn(),
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      await helpers.personalCharts()

      expect(mockQueryClient.cancelQueries).toHaveBeenCalledWith({
        queryKey: ['charts', 'personal'],
      })
      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['charts', 'personal'],
      })
    })

    it('restarts a pending personal fetch before stale data can land', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      const queryKey = ['charts', 'personal', '42'] as const
      const firstResponse = createDeferred<{ saved_shows: number }>()
      const queryFn = vi
        .fn<() => Promise<{ saved_shows: number }>>()
        .mockImplementationOnce(() => firstResponse.promise)
        .mockResolvedValue({ saved_shows: 1 })
      const observer = new QueryObserver(queryClient, { queryKey, queryFn })
      const unsubscribe = observer.subscribe(() => {})

      await vi.waitFor(() => expect(queryFn).toHaveBeenCalledTimes(1))
      await createInvalidateQueries(queryClient).personalCharts()

      expect(queryFn).toHaveBeenCalledTimes(2)
      expect(queryClient.getQueryData(queryKey)).toEqual({ saved_shows: 1 })

      firstResponse.resolve({ saved_shows: 0 })
      await Promise.resolve()
      expect(queryClient.getQueryData(queryKey)).toEqual({ saved_shows: 1 })
      unsubscribe()
    })

    it('creates invalidate helpers for mySubmissions', async () => {
      const { createInvalidateQueries } = await import('./queryClient')
      const mockQueryClient = {
        invalidateQueries: vi.fn(),
      } as unknown as QueryClient

      const helpers = createInvalidateQueries(mockQueryClient)
      helpers.mySubmissions()

      expect(mockQueryClient.invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['mySubmissions'],
      })
    })
  })

  describe('retry logic', () => {
    it('does not retry on 4xx errors other than 429', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const defaults = client.getDefaultOptions()
      const retryFn = defaults.queries?.retry as (
        failureCount: number,
        error: Error & { status?: number }
      ) => boolean

      // Simulate 400 error
      const error400 = Object.assign(new Error('Bad Request'), { status: 400 })
      expect(retryFn(0, error400)).toBe(false)

      // Simulate 404 error
      const error404 = Object.assign(new Error('Not Found'), { status: 404 })
      expect(retryFn(0, error404)).toBe(false)

      // Simulate 403 error
      const error403 = Object.assign(new Error('Forbidden'), { status: 403 })
      expect(retryFn(0, error403)).toBe(false)
    })

    it('retries up to 3 times for 5xx errors', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const defaults = client.getDefaultOptions()
      const retryFn = defaults.queries?.retry as (
        failureCount: number,
        error: Error & { status?: number }
      ) => boolean

      const error500 = Object.assign(new Error('Server Error'), { status: 500 })

      expect(retryFn(0, error500)).toBe(true) // First retry
      expect(retryFn(1, error500)).toBe(true) // Second retry
      expect(retryFn(2, error500)).toBe(true) // Third retry
      expect(retryFn(3, error500)).toBe(false) // No more retries
    })

    it('retries for network errors without status', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const defaults = client.getDefaultOptions()
      const retryFn = defaults.queries?.retry as (
        failureCount: number,
        error: Error & { status?: number }
      ) => boolean

      const networkError = new Error('Network error')

      expect(retryFn(0, networkError)).toBe(true)
      expect(retryFn(1, networkError)).toBe(true)
      expect(retryFn(2, networkError)).toBe(true)
      expect(retryFn(3, networkError)).toBe(false)
    })

    // PSY-1912: 429 is the one 4xx that means "ask again later". The default
    // policy used to lump it in with 400/403/404, so a transient rate limit
    // rendered as a permanently dead page block.
    it('retries a 429, unlike every other 4xx', async () => {
      const { getQueryClient } = await import('./queryClient')
      const defaults = getQueryClient().getDefaultOptions()
      const retryFn = defaults.queries?.retry as (
        failureCount: number,
        error: Error & { status?: number }
      ) => boolean

      const error429 = Object.assign(new Error('Too Many Requests'), {
        status: 429,
      })

      expect(retryFn(0, error429)).toBe(true)
      expect(retryFn(2, error429)).toBe(true)
      expect(retryFn(3, error429)).toBe(false)
    })

    it('wires a retryDelay so 429 waits are not the blind default curve', async () => {
      const { getQueryClient } = await import('./queryClient')
      const defaults = getQueryClient().getDefaultOptions()
      const retryDelay = defaults.queries?.retryDelay as (
        failureCount: number,
        error: Error & { status?: number; retryAfter?: number }
      ) => number

      const error429 = Object.assign(new Error('Too Many Requests'), {
        status: 429,
      })

      // React Query's own curve would put the first retry 1s out, well inside
      // the limiter window. The rate-limit schedule is deliberately slower.
      expect(retryDelay(0, error429)).toBeGreaterThanOrEqual(2_000)
      // Non-429 timing is untouched.
      expect(
        retryDelay(0, Object.assign(new Error('boom'), { status: 500 }))
      ).toBe(1_000)
    })
  })

  // End-to-end through a real QueryClient carrying the shared defaults, so
  // these cover the wiring as well as the policy: a predicate-only test would
  // still pass if `retry` were never attached to the client.
  describe('429 recovery through the shared client', () => {
    const rateLimited = () =>
      Object.assign(new Error('Too Many Requests'), { status: 429 })

    beforeEach(() => {
      vi.mocked(Sentry.captureMessage).mockClear()
      vi.useFakeTimers()
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('recovers when a 429 is followed by a 200', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const queryFn = vi
        .fn<() => Promise<{ ok: boolean }>>()
        .mockRejectedValueOnce(rateLimited())
        .mockResolvedValue({ ok: true })

      const result = client.fetchQuery({
        queryKey: ['artists', 'releases', 'recovers'],
        queryFn,
      })

      // One full limiter window covers the whole retry budget by design.
      await vi.advanceTimersByTimeAsync(60_000)

      await expect(result).resolves.toEqual({ ok: true })
      expect(queryFn).toHaveBeenCalledTimes(2)
      // A recovered 429 is not a user-visible failure, so nothing is reported
      // as exhausted. Visibility for it comes from the fetch-boundary hit
      // signal in lib/api.ts instead.
      expect(Sentry.captureMessage).not.toHaveBeenCalledWith(
        'Rate limit retries exhausted (HTTP 429)',
        expect.anything()
      )
    })

    it('surfaces the existing error state once a persistent 429 exhausts the budget', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()
      const queryKey = ['artists', 'releases', 'exhausts'] as const

      const queryFn = vi
        .fn<() => Promise<{ ok: boolean }>>()
        .mockRejectedValue(rateLimited())

      const result = client.fetchQuery({ queryKey, queryFn }).catch(e => e)

      await vi.advanceTimersByTimeAsync(60_000)

      const error = (await result) as { status?: number }
      expect(error.status).toBe(429)
      // Original request plus the three-retry budget.
      expect(queryFn).toHaveBeenCalledTimes(4)
      expect(client.getQueryState(queryKey)?.status).toBe('error')

      // The user saw a broken block, so this one reports at error level.
      expect(Sentry.captureMessage).toHaveBeenCalledWith(
        'Rate limit retries exhausted (HTTP 429)',
        expect.objectContaining({
          level: 'error',
          extra: expect.objectContaining({
            queryFamily: 'artists/releases',
            attempts: 4,
          }),
        })
      )
    })

    it('does not retry or report a non-429 4xx', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()

      const queryFn = vi
        .fn<() => Promise<unknown>>()
        .mockRejectedValue(
          Object.assign(new Error('Not Found'), { status: 404 })
        )

      const result = client
        .fetchQuery({ queryKey: ['artists', 'detail', 'missing'], queryFn })
        .catch(e => e)

      await vi.advanceTimersByTimeAsync(60_000)

      expect(((await result) as { status?: number }).status).toBe(404)
      expect(queryFn).toHaveBeenCalledTimes(1)
      expect(Sentry.captureMessage).not.toHaveBeenCalled()
    })
  })
})
