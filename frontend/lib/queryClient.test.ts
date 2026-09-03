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

    // PSY-1912. The POLICY itself is covered in query-retry-policy.test.ts;
    // what has to be asserted here is only that the client actually uses it,
    // since a policy nobody wired up would leave 429s terminal exactly as
    // before. Identity checks rather than behavioural re-testing, so this
    // cannot drift into a second, weaker copy of those assertions.
    it('wires the shared retry policy onto the client', async () => {
      const { getQueryClient } = await import('./queryClient')
      const { shouldRetryQuery, queryRetryDelay } = await import(
        './query-retry-policy'
      )

      const defaults = getQueryClient().getDefaultOptions()

      expect(defaults.queries?.retry).toBe(shouldRetryQuery)
      expect(defaults.queries?.retryDelay).toBe(queryRetryDelay)
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

    // A server-side 429 is terminal by design, so it reaches the cache's
    // onError on its FIRST failure. Without the browser gate it would report
    // "retries exhausted" at error level having attempted none: the loudest
    // signal in the system firing for the case the policy calls harmless,
    // and the common case at that, since every SSR render from one instance
    // shares an egress IP against an IP-keyed limiter.
    it('does not report an exhausted rate limit outside the browser', async () => {
      const { reportRateLimitExhausted } = await import(
        './rate-limit-telemetry'
      )
      const windowSpy = vi
        .spyOn(globalThis, 'window', 'get')
        .mockReturnValue(undefined as unknown as Window & typeof globalThis)

      try {
        const { getQueryClient } = await import('./queryClient')
        const client = getQueryClient()
        const queryFn = vi
          .fn<() => Promise<unknown>>()
          .mockRejectedValue(rateLimited())

        const result = client
          .fetchQuery({ queryKey: ['artists', 'releases', 'ssr'], queryFn })
          .catch(e => e)
        await vi.advanceTimersByTimeAsync(60_000)
        await result

        // Terminal on the server: one attempt, no retries.
        expect(queryFn).toHaveBeenCalledTimes(1)
        expect(Sentry.captureMessage).not.toHaveBeenCalledWith(
          'Rate limit retries exhausted (HTTP 429)',
          expect.anything()
        )
      } finally {
        windowSpy.mockRestore()
      }

      // The reporter itself is unchanged; it is the query-cache caller that
      // declines to invoke it on the server.
      expect(typeof reportRateLimitExhausted).toBe('function')
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
      // Scoped to the rate-limit message rather than the whole Sentry
      // surface, so unrelated reporting added later cannot fail this test for
      // the wrong reason.
      expect(Sentry.captureMessage).not.toHaveBeenCalledWith(
        'Rate limit retries exhausted (HTTP 429)',
        expect.anything()
      )
    })
  })

  // PSY-1946. Session expiry has no logout, so a 401 is the only signal that
  // the privileged payloads on screen belong to a session that has ended.
  describe('session expiry re-masks viewer-tier caches', () => {
    const expired = () =>
      Object.assign(new Error('Unauthorized'), { status: 401 })

    // The state a viewer is in when their session dies mid-view: the profile
    // query still holds the payload that named them, because TanStack retains
    // the last success when a refetch errors.
    function seedSignedInProfile(client: {
      setQueryData: (key: readonly unknown[], data: unknown) => void
    }) {
      client.setQueryData(['auth', 'profile'], {
        success: true,
        user: { id: 6, email: 'alice@example.com' },
      })
    }

    // The families a reset touched, in call order. Written once because the
    // cast and the key-shape assumption are the same in every assertion below.
    function resetFamilies(spy: { mock: { calls: unknown[][] } }): unknown[] {
      return spy.mock.calls.map(
        ([options]) => (options as { queryKey: unknown[] }).queryKey[0]
      )
    }

    async function fail401(
      client: {
        fetchQuery: (options: {
          queryKey: readonly unknown[]
          queryFn: () => Promise<unknown>
          retry: boolean
          staleTime: number
        }) => Promise<unknown>
      },
      queryKey: readonly unknown[]
    ) {
      await client
        .fetchQuery({
          queryKey,
          queryFn: () => Promise.reject(expired()),
          retry: false,
          // `fetchQuery` honours the 15-minute default, so a key that already
          // carries data resolves from cache and never reaches the handler.
          // The profile key is exactly that key here.
          staleTime: 0,
        })
        .catch(() => undefined)
    }

    it('resets the viewer-tier families once for a burst of 401s', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()
      seedSignedInProfile(client)

      const resetQueries = vi.spyOn(client, 'resetQueries')

      // An entity page's parallel reads all 401 together when a session ends.
      await fail401(client, ['revisions', 'entity', 'venues', 1])
      await fail401(client, ['comments', 'entity', 'venues', 1])
      await fail401(client, ['collections', 'entity', 'venues', 1])

      const families = resetFamilies(resetQueries)
      expect(families).toContain('revisions')
      expect(families).toContain('comments')
      expect(families).toContain('collections')

      // One reset PER FAMILY, from one episode. Unlatched, the refetch each
      // reset starts would 401 with the same dead cookie and reset again.
      expect(families.filter(f => f === 'revisions')).toHaveLength(1)
    })

    it('does not reset again when the reset\'s own refetch 401s', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()
      seedSignedInProfile(client)

      await fail401(client, ['revisions', 'entity', 'venues', 1])
      const resetQueries = vi.spyOn(client, 'resetQueries')

      // The refetch the reset above started, answered the same way.
      await fail401(client, ['revisions', 'entity', 'venues', 1])

      expect(resetQueries).not.toHaveBeenCalled()
    })

    it('spends no reset on an anonymous viewer whose request was refused', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()
      // The sentinel the SSR prefetch seeds for a viewer with no session.
      client.setQueryData(['auth', 'profile'], {
        success: false,
        error_code: 'TOKEN_MISSING',
      })
      const resetQueries = vi.spyOn(client, 'resetQueries')

      await fail401(client, ['revisions', 'entity', 'venues', 1])

      expect(resetQueries).not.toHaveBeenCalled()
    })

    it('re-arms on the next session entry', async () => {
      const { getQueryClient, refreshViewerTierQueries } = await import(
        './queryClient'
      )
      const client = getQueryClient()
      seedSignedInProfile(client)

      await fail401(client, ['revisions', 'entity', 'venues', 1])

      // A new session: the next expiry is a new episode, not an echo.
      await refreshViewerTierQueries(client)
      seedSignedInProfile(client)
      const resetQueries = vi.spyOn(client, 'resetQueries')

      await fail401(client, ['revisions', 'entity', 'venues', 1])

      expect(resetQueries).toHaveBeenCalled()
    })

    it('resets on the profile query own 401, which is the settle', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()
      seedSignedInProfile(client)
      const resetQueries = vi.spyOn(client, 'resetQueries')

      await fail401(client, ['auth', 'profile'])

      expect(resetFamilies(resetQueries)).toContain('revisions')
    })

    it('leaves the profile query itself out of the reset', async () => {
      const { getQueryClient } = await import('./queryClient')
      const client = getQueryClient()
      seedSignedInProfile(client)
      const resetQueries = vi.spyOn(client, 'resetQueries')

      await fail401(client, ['revisions', 'entity', 'venues', 1])

      // AuthContext reads the profile's definitive 401 as 'anonymous' ahead of
      // any retained payload, so its error IS the settle; resetting it would
      // discard the answer.
      expect(resetFamilies(resetQueries)).not.toContain('auth')
    })
  })
})
