import { describe, it, expect, vi, beforeAll, beforeEach, afterAll } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'
import { server } from '@/test/mocks/server'

// This test file mocks global.fetch directly (these hooks use raw fetch,
// not apiRequest). We must close the MSW server for this file to prevent
// MSW's fetch interceptor from interfering with the mock.
const mockFetch = vi.fn()
let originalFetch: typeof globalThis.fetch
beforeAll(() => {
  server.close()
  originalFetch = globalThis.fetch
  globalThis.fetch = mockFetch
})
afterAll(() => {
  globalThis.fetch = originalFetch
  // PSY-945: restore the GLOBAL unhandled-request policy ('error', set in
  // test/setup.ts). Re-opening with 'bypass' here would silently re-arm the
  // pass-through-to-real-network behavior for every test file that runs after
  // this one in the same worker — the exact leak that caused the intermittent
  // "Closing rpc while fetch was pending" teardown flake.
  server.listen({ onUnhandledRequest: 'error' })
})

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    artists: {
      all: ['artists'],
      detail: (id: number) => ['artists', 'detail', id],
      aliases: (id: number) => ['artists', 'aliases', id],
    },
    shows: {
      all: ['shows'],
    },
  },
}))

// Import hooks after mocks are set up
import {
  useDiscoverMusic,
  useUpdateArtistBandcamp,
  useClearArtistBandcamp,
  useUpdateArtistSpotify,
  useClearArtistSpotify,
  useArtistUpdate,
  useArtistAliases,
  useCreateArtistAlias,
  useDeleteArtistAlias,
  useMergeArtists,
  type DiscoverMusicResponse,
} from './useAdminArtists'


// Helper to mock successful fetch response. Includes a Headers-like
// object so `apiRequest`'s `response.headers.get(...)` call doesn't
// blow up for hooks that go through apiRequest (vs raw fetch).
function mockFetchResponse(data: unknown, ok = true, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok,
    status,
    headers: new Headers(),
    json: async () => data,
  })
}

// Helper to mock failed fetch response
function mockFetchError(message: string, status = 500) {
  mockFetch.mockResolvedValueOnce({
    ok: false,
    status,
    headers: new Headers(),
    json: async () => ({ message, error: message }),
  })
}

describe('useAdminArtists', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetch.mockReset()
  })

  describe('useDiscoverMusic', () => {
    it('POSTs to discover-music and returns candidates per platform', async () => {
      // Response is { bandcamp: [...], spotify: [...] } with disambiguation
      // metadata per candidate; nothing is saved by this call.
      const mockResponse: DiscoverMusicResponse = {
        bandcamp: [
          {
            url: 'https://wednesdayband.bandcamp.com/album/bleeds',
            name_as_listed: 'Wednesday',
            location: 'Asheville, NC',
            notable_release: 'Bleeds (2025)',
            genres: 'shoegaze, indie',
            popularity: null,
            confidence: 'high',
            why_might_match: 'Primary Asheville band.',
          },
        ],
        spotify: [
          {
            url: 'https://open.spotify.com/artist/5IjZr8fAPiOAr7NQj5wZaQ',
            name_as_listed: 'Wednesday',
            location: 'Asheville, NC',
            notable_release: null,
            genres: null,
            popularity: '3K monthly listeners',
            confidence: 'high',
            why_might_match: null,
          },
        ],
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useDiscoverMusic(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/admin/artists/123/discover-music',
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
        })
      )
      expect(result.current.data?.bandcamp).toHaveLength(1)
      expect(result.current.data?.spotify).toHaveLength(1)
    })

    it('handles discovery failure', async () => {
      mockFetchError('Discovery is rate-limited right now — try again in about a minute.', 429)

      const { result } = renderHook(() => useDiscoverMusic(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(999)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe(
        'Discovery is rate-limited right now — try again in about a minute.'
      )
    })

    it('maps the client-side AbortSignal timeout to a friendly message', async () => {
      // AbortSignal.timeout rejects fetch with a TimeoutError DOMException; the
      // hook converts that to actionable copy instead of leaking the raw error.
      mockFetch.mockRejectedValueOnce(
        new DOMException('The operation timed out', 'TimeoutError')
      )

      const { result } = renderHook(() => useDiscoverMusic(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe(
        'Discovery timed out — try again, or use manual entry.'
      )
    })
  })

  describe('useUpdateArtistBandcamp', () => {
    it('updates artist Bandcamp URL', async () => {
      const mockResponse = {
        success: true,
        artist: { id: 123, bandcamp_url: 'https://newartist.bandcamp.com' },
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useUpdateArtistBandcamp(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 123,
          bandcampUrl: 'https://newartist.bandcamp.com',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/admin/artists/123/bandcamp',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({
            bandcamp_url: 'https://newartist.bandcamp.com',
          }),
        })
      )
    })

    it('invalidates artist query on success', async () => {
      mockFetchResponse({ success: true })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useUpdateArtistBandcamp(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 456,
          bandcampUrl: 'https://test.bandcamp.com',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Invalidates the artists prefix (matches the slug-keyed cache), not an
      // id-keyed detail key (which holds a different value than the slug).
      // (PSY-1109)
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists'],
      })
    })

    it('handles update failure', async () => {
      mockFetchError('Update failed', 400)

      const { result } = renderHook(() => useUpdateArtistBandcamp(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 123,
          bandcampUrl: 'invalid-url',
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Update failed')
    })
  })

  describe('useClearArtistBandcamp', () => {
    it('clears artist Bandcamp URL', async () => {
      const mockResponse = {
        success: true,
        artist: { id: 123, bandcamp_url: null as string | null },
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useClearArtistBandcamp(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/admin/artists/123/bandcamp',
        expect.objectContaining({
          method: 'DELETE',
          credentials: 'include',
        })
      )
    })

    it('invalidates artist query on success', async () => {
      mockFetchResponse({ success: true })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useClearArtistBandcamp(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate(789)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists'],
      })
    })

    it('handles clear failure', async () => {
      mockFetchError('Clear failed', 500)

      const { result } = renderHook(() => useClearArtistBandcamp(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Clear failed')
    })
  })

  describe('useUpdateArtistSpotify', () => {
    it('updates artist Spotify URL', async () => {
      const mockResponse = {
        success: true,
        artist: {
          id: 123,
          spotify_url: 'https://open.spotify.com/artist/abc123',
        },
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useUpdateArtistSpotify(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 123,
          spotifyUrl: 'https://open.spotify.com/artist/abc123',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/admin/artists/123/spotify',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({
            spotify_url: 'https://open.spotify.com/artist/abc123',
          }),
        })
      )
    })

    it('invalidates artist query on success', async () => {
      mockFetchResponse({ success: true })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useUpdateArtistSpotify(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 456,
          spotifyUrl: 'https://open.spotify.com/artist/test',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Invalidates the artists prefix (matches the slug-keyed cache), not an
      // id-keyed detail key (which holds a different value than the slug).
      // (PSY-1109)
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists'],
      })
    })

    it('handles update failure', async () => {
      mockFetchError('Update failed', 400)

      const { result } = renderHook(() => useUpdateArtistSpotify(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 123,
          spotifyUrl: 'invalid-url',
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Update failed')
    })
  })

  describe('useClearArtistSpotify', () => {
    it('clears artist Spotify URL', async () => {
      const mockResponse = {
        success: true,
        artist: { id: 123, spotify_url: null as string | null },
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useClearArtistSpotify(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/admin/artists/123/spotify',
        expect.objectContaining({
          method: 'DELETE',
          credentials: 'include',
        })
      )
    })

    it('invalidates artist query on success', async () => {
      mockFetchResponse({ success: true })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useClearArtistSpotify(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate(789)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists'],
      })
    })

    it('handles clear failure', async () => {
      mockFetchError('Clear failed', 500)

      const { result } = renderHook(() => useClearArtistSpotify(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Clear failed')
    })
  })

  describe('useArtistUpdate', () => {
    it('PATCHes the admin artist endpoint with the edit payload', async () => {
      // useArtistUpdate goes through apiRequest (not raw fetch), so the
      // full http://localhost:8080 base is included in the URL.
      mockFetchResponse({ id: 123, name: 'Renamed' })

      const { result } = renderHook(() => useArtistUpdate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 123,
          data: { name: 'Renamed', city: 'Phoenix' },
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const [url, init] = mockFetch.mock.calls[0]
      expect(url).toMatch(/\/admin\/artists\/123$/)
      expect(init).toMatchObject({
        method: 'PATCH',
        body: JSON.stringify({ name: 'Renamed', city: 'Phoenix' }),
      })
    })

    it('invalidates the artists.all AND shows.all prefixes on success', async () => {
      // Updating an artist may rename them — show listings carry the artist
      // name denormalised, so the shows scope also gets invalidated. (No
      // detail(id) line — the artists prefix covers detail-by-slug. PSY-1109.)
      mockFetchResponse({ id: 456 })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useArtistUpdate(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 456,
          data: { name: 'New Name' },
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists'],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['shows'],
      })
    })

    it('surfaces update failures', async () => {
      mockFetchError('Validation failed', 422)

      const { result } = renderHook(() => useArtistUpdate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          artistId: 123,
          data: { name: '' },
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Validation failed')
    })
  })

  describe('useArtistAliases', () => {
    it('fetches aliases for a positive artist id', async () => {
      mockFetchResponse({ aliases: [{ id: 1, alias: 'X' }] })

      const { result } = renderHook(() => useArtistAliases(42), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      const [url] = mockFetch.mock.calls[0]
      expect(url).toMatch(/\/artists\/42\/aliases$/)
    })

    it('does not fetch when enabled=false', () => {
      const { result } = renderHook(() => useArtistAliases(42, false), {
        wrapper: createWrapper(),
      })

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('does not fetch when artistId is 0', () => {
      const { result } = renderHook(() => useArtistAliases(0), {
        wrapper: createWrapper(),
      })

      expect(result.current.fetchStatus).toBe('idle')
    })
  })

  describe('useCreateArtistAlias', () => {
    it('POSTs the alias body and invalidates that artist’s aliases', async () => {
      mockFetchResponse({ id: 1, artist_id: 42, alias: 'New Alias' })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useCreateArtistAlias(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ artistId: 42, alias: 'New Alias' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const [url, init] = mockFetch.mock.calls[0]
      expect(url).toMatch(/\/admin\/artists\/42\/aliases$/)
      expect(init).toMatchObject({
        method: 'POST',
        body: JSON.stringify({ alias: 'New Alias' }),
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists', 'aliases', 42],
      })
    })
  })

  describe('useDeleteArtistAlias', () => {
    it('DELETEs and invalidates the artist’s aliases', async () => {
      mockFetchResponse(undefined)

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useDeleteArtistAlias(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ artistId: 42, aliasId: 7 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const [url, init] = mockFetch.mock.calls[0]
      expect(url).toMatch(/\/admin\/artists\/42\/aliases\/7$/)
      expect(init).toMatchObject({ method: 'DELETE' })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists', 'aliases', 42],
      })
    })
  })

  describe('useMergeArtists', () => {
    it('POSTs canonical + merge-from ids and invalidates artists + shows', async () => {
      // Merge collapses two artists into one. Anywhere they appear (shows,
      // listings, references) must refetch — guard the broad invalidation.
      mockFetchResponse({
        canonical_artist_id: 10,
        merged_count: 5,
      })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useMergeArtists(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({
          canonicalArtistId: 10,
          mergeFromArtistId: 20,
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const [url, init] = mockFetch.mock.calls[0]
      expect(url).toMatch(/\/admin\/artists\/merge$/)
      expect(init).toMatchObject({
        method: 'POST',
        body: JSON.stringify({
          canonical_artist_id: 10,
          merge_from_artist_id: 20,
        }),
      })

      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['artists'] })
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['shows'] })
    })

    it('surfaces merge errors', async () => {
      mockFetchError('Cannot merge with self', 400)

      const { result } = renderHook(() => useMergeArtists(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          canonicalArtistId: 10,
          mergeFromArtistId: 10,
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Cannot merge with self')
    })
  })
})
