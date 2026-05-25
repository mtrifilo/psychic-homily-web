import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      STREAMING_WORKLIST: {
        LIST: '/admin/streaming-worklist',
      },
      ARTISTS: {
        STREAMING_DISCOVERY_STATUS: (artistId: string | number) =>
          `/admin/artists/${artistId}/streaming-discovery-status`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      streamingWorklist: (params?: Record<string, unknown>) => [
        'admin',
        'streamingWorklist',
        params,
      ],
    },
  },
}))

import {
  useStreamingWorklist,
  useUpdateStreamingDiscoveryStatus,
} from './useStreamingWorklist'

function setupClient() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  return { queryClient, invalidateSpy }
}

describe('useStreamingWorklist', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('GETs the list endpoint without a status when filter is empty', async () => {
    mockApiRequest.mockResolvedValueOnce({ entries: [], total: 0 })
    const { queryClient } = setupClient()

    const { result } = renderHook(
      () => useStreamingWorklist({ status: '', limit: 25, offset: 0 }),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/streaming-worklist?limit=25&offset=0'
    )
    expect(result.current.data).toEqual({ entries: [], total: 0 })
  })

  it('forwards the status filter as a query-string param', async () => {
    mockApiRequest.mockResolvedValueOnce({ entries: [], total: 0 })
    const { queryClient } = setupClient()

    renderHook(
      () =>
        useStreamingWorklist({
          status: 'unreviewed',
          limit: 25,
          offset: 0,
        }),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    await waitFor(() =>
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/streaming-worklist?limit=25&offset=0&status=unreviewed'
      )
    )
  })

  it('skips fetching when disabled', () => {
    const { queryClient } = setupClient()

    renderHook(
      () =>
        useStreamingWorklist({
          status: '',
          limit: 25,
          offset: 0,
          enabled: false,
        }),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})

describe('useUpdateStreamingDiscoveryStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('POSTs the status mutation and invalidates the worklist branch', async () => {
    mockApiRequest.mockResolvedValueOnce({
      body: {
        id: 42,
        name: 'Test Band',
        slug: 'test-band',
        streaming_discovery_status: 'skipped',
        streaming_discovery_reason: 'Same-name collision',
        updated_at: '2026-05-24T12:00:00Z',
      },
    })
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useUpdateStreamingDiscoveryStatus(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({
        artist_id: 42,
        status: 'skipped',
        reason: 'Same-name collision',
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/artists/42/streaming-discovery-status',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          status: 'skipped',
          reason: 'Same-name collision',
        }),
      })
    )
    // Whole worklist branch must invalidate so every status-filter +
    // pagination combination refetches and the row drops out.
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['admin', 'streamingWorklist'],
    })
  })

  it('passes null reason through unchanged', async () => {
    mockApiRequest.mockResolvedValueOnce({
      body: {
        id: 7,
        name: 'X',
        streaming_discovery_status: 'linked',
        updated_at: '2026-05-24T12:00:00Z',
      },
    })
    const { queryClient } = setupClient()

    const { result } = renderHook(() => useUpdateStreamingDiscoveryStatus(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({
        artist_id: 7,
        status: 'linked',
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/artists/7/streaming-discovery-status',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ status: 'linked', reason: null }),
      })
    )
  })

  it('unwraps the Huma `body` envelope into the mutation result', async () => {
    const updatedArtist = {
      id: 9,
      name: 'Y',
      streaming_discovery_status: 'no_links_found' as const,
      updated_at: '2026-05-24T12:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce({ body: updatedArtist })
    const { queryClient } = setupClient()

    const { result } = renderHook(() => useUpdateStreamingDiscoveryStatus(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    let returned: unknown
    await act(async () => {
      returned = await result.current.mutateAsync({
        artist_id: 9,
        status: 'no_links_found',
      })
    })

    expect(returned).toEqual(updatedArtist)
  })
})
