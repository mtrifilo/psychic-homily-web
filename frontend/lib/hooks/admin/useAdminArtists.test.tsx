import { describe, it, expect, vi, beforeAll, beforeEach, afterAll } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
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
  server.listen({ onUnhandledRequest: 'bypass' })
})

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    artists: {
      detail: (id: number) => ['artists', 'detail', id],
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
} from './useAdminArtists'


// Helper to mock successful fetch response
function mockFetchResponse(data: unknown, ok = true, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok,
    status,
    json: async () => data,
  })
}

// Helper to mock failed fetch response
function mockFetchError(message: string, status = 500) {
  mockFetch.mockResolvedValueOnce({
    ok: false,
    status,
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
      const mockResponse = {
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

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists', 'detail', 456],
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
        queryKey: ['artists', 'detail', 789],
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

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists', 'detail', 456],
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
        queryKey: ['artists', 'detail', 789],
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
})
