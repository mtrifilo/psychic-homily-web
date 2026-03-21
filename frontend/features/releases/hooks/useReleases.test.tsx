import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    RELEASES: {
      LIST: '/releases',
      GET: (idOrSlug: string | number) => `/releases/${idOrSlug}`,
      ARTIST_RELEASES: (artistIdOrSlug: string | number) => `/artists/${artistIdOrSlug}/releases`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    releases: {
      list: (filters?: Record<string, unknown>) => ['releases', 'list', filters],
      detail: (id: string | number) => ['releases', 'detail', String(id)],
      artistReleases: (id: string | number) => ['releases', 'artist', String(id)],
    },
  },
}))

import { useReleases, useRelease, useArtistReleases } from './useReleases'


describe('useReleases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches releases without filters', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], count: 0 })

    const { result } = renderHook(() => useReleases(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/releases', { method: 'GET' })
  })

  it('includes releaseType filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], count: 0 })

    const { result } = renderHook(() => useReleases({ releaseType: 'album' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('release_type=album')
  })

  it('includes year filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], count: 0 })

    const { result } = renderHook(() => useReleases({ year: 2025 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('year=2025')
  })

  it('includes artistId filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], count: 0 })

    const { result } = renderHook(() => useReleases({ artistId: 42 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('artist_id=42')
  })

  it('handles multiple filters', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], count: 0 })

    const { result } = renderHook(
      () => useReleases({ releaseType: 'ep', year: 2024, artistId: 5 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('release_type=ep')
    expect(url).toContain('year=2024')
    expect(url).toContain('artist_id=5')
  })
})

describe('useRelease', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a release by slug', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'OK Computer', slug: 'ok-computer' })

    const { result } = renderHook(() => useRelease({ idOrSlug: 'ok-computer' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/releases/ok-computer', { method: 'GET' })
  })

  it('fetches a release by numeric ID', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'Test' })

    const { result } = renderHook(() => useRelease({ idOrSlug: 42 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/releases/42', { method: 'GET' })
  })

  it('does not fetch when idOrSlug is 0', () => {
    const { result } = renderHook(() => useRelease({ idOrSlug: 0 }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when idOrSlug is empty string', () => {
    const { result } = renderHook(() => useRelease({ idOrSlug: '' }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useRelease({ idOrSlug: 'test', enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('handles API errors', async () => {
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRelease({ idOrSlug: 'nonexistent' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useArtistReleases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches releases for an artist', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [{ id: 1, title: 'Album' }] })

    const { result } = renderHook(
      () => useArtistReleases({ artistIdOrSlug: 'radiohead' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/radiohead/releases', { method: 'GET' })
  })

  it('does not fetch when artistIdOrSlug is 0', () => {
    const { result } = renderHook(
      () => useArtistReleases({ artistIdOrSlug: 0 }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when artistIdOrSlug is empty string', () => {
    const { result } = renderHook(
      () => useArtistReleases({ artistIdOrSlug: '' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})
