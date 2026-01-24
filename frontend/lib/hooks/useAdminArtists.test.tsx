import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mock for fetch
const mockFetch = vi.fn()
global.fetch = mockFetch

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    artists: {
      detail: (id: number) => ['artists', 'detail', id],
    },
  },
}))

// Import hooks after mocks are set up
import {
  useDiscoverMusic,
  useDiscoverBandcamp,
  useUpdateArtistBandcamp,
  useClearArtistBandcamp,
  useUpdateArtistSpotify,
  useClearArtistSpotify,
} from './useAdminArtists'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

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
    it('discovers music for an artist and returns Bandcamp URL', async () => {
      const mockResponse = {
        success: true,
        platform: 'bandcamp',
        url: 'https://artist.bandcamp.com/album/test',
        discovered_url: 'https://artist.bandcamp.com/album/test',
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
      expect(result.current.data?.platform).toBe('bandcamp')
      expect(result.current.data?.url).toBe(
        'https://artist.bandcamp.com/album/test'
      )
    })

    it('discovers music and returns Spotify URL', async () => {
      const mockResponse = {
        success: true,
        platform: 'spotify',
        url: 'https://open.spotify.com/artist/abc123',
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useDiscoverMusic(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(456)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.platform).toBe('spotify')
    })

    it('invalidates artist query on success', async () => {
      mockFetchResponse({ success: true, platform: 'bandcamp' })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useDiscoverMusic(), {
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

    it('handles discovery failure', async () => {
      mockFetchError('Artist not found on any platform', 404)

      const { result } = renderHook(() => useDiscoverMusic(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(999)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe(
        'Artist not found on any platform'
      )
    })

    it('handles server error', async () => {
      mockFetchError('Discovery failed', 500)

      const { result } = renderHook(() => useDiscoverMusic(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useDiscoverBandcamp', () => {
    it('discovers Bandcamp for an artist', async () => {
      const mockResponse = {
        success: true,
        bandcamp_url: 'https://artist.bandcamp.com/album/test',
        discovered_url: 'https://artist.bandcamp.com/album/test',
      }
      mockFetchResponse(mockResponse)

      const { result } = renderHook(() => useDiscoverBandcamp(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/admin/artists/123/discover-bandcamp',
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
        })
      )
      expect(result.current.data?.bandcamp_url).toBe(
        'https://artist.bandcamp.com/album/test'
      )
    })

    it('invalidates artist query on success', async () => {
      mockFetchResponse({ success: true })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useDiscoverBandcamp(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate(456)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['artists', 'detail', 456],
      })
    })

    it('handles discovery failure', async () => {
      mockFetchError('No Bandcamp found', 404)

      const { result } = renderHook(() => useDiscoverBandcamp(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(999)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
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
        artist: { id: 123, bandcamp_url: null },
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
        artist: { id: 123, spotify_url: null },
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
