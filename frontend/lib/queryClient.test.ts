import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'

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
    it('generates auth profile key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.auth.profile).toEqual(['auth', 'profile'])
    })

    it('generates auth user key with id', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.auth.user('user-123')).toEqual([
        'auth',
        'user',
        'user-123',
      ])
    })

    it('generates shows list key with filters', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.shows.list({ city: 'Phoenix' })).toEqual([
        'shows',
        'list',
        { city: 'Phoenix' },
      ])
    })

    it('generates shows detail key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.shows.detail('show-456')).toEqual([
        'shows',
        'detail',
        'show-456',
      ])
    })

    it('generates venues search key with lowercase query', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.venues.search('REBEL LOUNGE')).toEqual([
        'venues',
        'search',
        'rebel lounge',
      ])
    })

    it('generates venues cities key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.venues.cities).toEqual(['venues', 'cities'])
    })

    it('generates venues shows key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.venues.shows(42)).toEqual(['venues', 'shows', '42'])
    })

    it('generates venues myPendingEdit key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.venues.myPendingEdit(10)).toEqual([
        'venues',
        'myPendingEdit',
        '10',
      ])
    })

    it('generates admin pendingVenueEdits key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.admin.pendingVenueEdits(20, 0)).toEqual([
        'admin',
        'venues',
        'pendingEdits',
        { limit: 20, offset: 0 },
      ])
    })

    it('generates artists search key with lowercase', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.artists.search('THE BAND')).toEqual([
        'artists',
        'search',
        'the band',
      ])
    })

    it('generates artists detail key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.artists.detail(99)).toEqual(['artists', 'detail', '99'])
    })

    it('generates artists shows key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.artists.shows(5)).toEqual(['artists', 'shows', '5'])
    })

    it('generates savedShows check key with string conversion', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.savedShows.check(123)).toEqual([
        'savedShows',
        'check',
        '123',
      ])
    })

    it('generates mySubmissions list key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.mySubmissions.list()).toEqual(['mySubmissions', 'list'])
    })

    it('generates system health key', async () => {
      const { queryKeys } = await import('./queryClient')

      expect(queryKeys.system.health).toEqual(['system', 'health'])
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
    it('does not retry on 4xx errors', async () => {
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
  })
})
