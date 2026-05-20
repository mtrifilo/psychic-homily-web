import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/artists/api', () => ({
  artistEndpoints: {
    GRAPH: (artistId: string | number) => `/artists/${artistId}/graph`,
    RELATIONSHIPS: {
      CREATE: '/artists/relationships',
      VOTE: (sourceId: number, targetId: number) =>
        `/artists/relationships/${sourceId}/${targetId}/vote`,
    },
  },
  artistQueryKeys: {
    graph: (id: string | number, types?: string[]) => [
      'artists',
      'graph',
      String(id),
      types,
    ],
  },
}))

// Import hooks after mocks are set up
import {
  useArtistGraph,
  useArtistRelationshipVote,
  useCreateArtistRelationship,
} from './useArtistGraph'

const mockGraph = {
  center: {
    id: 1,
    name: 'Center Artist',
    slug: 'center-artist',
    upcoming_show_count: 3,
  },
  nodes: [
    { id: 2, name: 'Related A', slug: 'related-a', upcoming_show_count: 1 },
  ],
  links: [
    {
      source_id: 1,
      target_id: 2,
      type: 'shared_bills',
      score: 0.8,
      votes_up: 4,
      votes_down: 0,
    },
  ],
}

describe('useArtistGraph', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the relationship graph for an artist', async () => {
    mockApiRequest.mockResolvedValueOnce(mockGraph)

    const { result } = renderHook(() => useArtistGraph({ artistId: 1 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/1/graph', {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockGraph)
  })

  it('forwards the types filter as a comma-joined query param', async () => {
    mockApiRequest.mockResolvedValueOnce(mockGraph)

    const { result } = renderHook(
      () => useArtistGraph({ artistId: 1, types: ['shared_bills', 'same_label'] }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest.mock.calls[0][0]).toBe(
      '/artists/1/graph?types=shared_bills%2Csame_label'
    )
  })

  it('starts in a loading state before the request resolves', () => {
    // Never-resolving promise keeps the query pending so we can assert the
    // loading branch the graph UI renders a spinner for.
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))

    const { result } = renderHook(() => useArtistGraph({ artistId: 1 }), {
      wrapper: createWrapper(),
    })

    expect(result.current.isLoading).toBe(true)
  })

  it('exposes an error when the graph fetch fails', async () => {
    const error = new Error('Artist not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useArtistGraph({ artistId: 999 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Artist not found')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useArtistGraph({ artistId: 1, enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when artistId is 0 or negative', () => {
    const { result: result0 } = renderHook(
      () => useArtistGraph({ artistId: 0 }),
      { wrapper: createWrapper() }
    )
    const { result: resultNeg } = renderHook(
      () => useArtistGraph({ artistId: -1 }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result0.current.fetchStatus).toBe('idle')
    expect(resultNeg.current.fetchStatus).toBe('idle')
  })
})

describe('useArtistRelationshipVote', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs the vote and invalidates the center artist graph', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useArtistRelationshipVote(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({
        sourceId: 1,
        targetId: 2,
        type: 'shared_bills',
        isUpvote: true,
        centerArtistId: 1,
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/relationships/1/2/vote',
      {
        method: 'POST',
        body: JSON.stringify({ type: 'shared_bills', is_upvote: true }),
      }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['artists', 'graph', '1'],
    })
  })

  it('surfaces an error and skips invalidation when the vote fails', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useArtistRelationshipVote(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          sourceId: 1,
          targetId: 2,
          type: 'shared_bills',
          isUpvote: false,
          centerArtistId: 1,
        })
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Unauthorized')
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})

describe('useCreateArtistRelationship', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs the new relationship and invalidates the center artist graph', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCreateArtistRelationship(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({
        sourceArtistId: 1,
        targetArtistId: 2,
        type: 'shared_bills',
        centerArtistId: 1,
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/relationships', {
      method: 'POST',
      body: JSON.stringify({
        source_artist_id: 1,
        target_artist_id: 2,
        type: 'shared_bills',
      }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['artists', 'graph', '1'],
    })
  })

  it('surfaces a duplicate-relationship error', async () => {
    const error = new Error('relationship already exists')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCreateArtistRelationship(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          sourceArtistId: 1,
          targetArtistId: 2,
          type: 'shared_bills',
          centerArtistId: 1,
        })
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe(
      'relationship already exists'
    )
  })
})
