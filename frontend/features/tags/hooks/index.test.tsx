import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    TAGS: {
      LIST: '/tags',
      SEARCH: '/tags/search',
      GET: (idOrSlug: string | number) => `/tags/${idOrSlug}`,
      ALIASES: (idOrSlug: string | number) => `/tags/${idOrSlug}/aliases`,
    },
    ENTITY_TAGS: {
      LIST: (entityType: string, entityId: number) => `/entities/${entityType}/${entityId}/tags`,
      ADD: (entityType: string, entityId: number) => `/entities/${entityType}/${entityId}/tags`,
      REMOVE: (entityType: string, entityId: number, tagId: number) =>
        `/entities/${entityType}/${entityId}/tags/${tagId}`,
      VOTE: (tagId: number, entityType: string, entityId: number) =>
        `/tags/${tagId}/entities/${entityType}/${entityId}/votes`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    tags: {
      all: ['tags'],
      list: (params?: Record<string, unknown>) => ['tags', 'list', params],
      search: (query: string) => ['tags', 'search', query.toLowerCase()],
      detail: (id: string | number) => ['tags', 'detail', String(id)],
      entityTags: (entityType: string, entityId: number) => ['tags', 'entityTags', entityType, entityId],
    },
  },
}))

import {
  useTags,
  useSearchTags,
  useTag,
  useEntityTags,
  useAddTagToEntity,
  useRemoveTagFromEntity,
  useVoteOnTag,
  useRemoveTagVote,
} from './index'

function createWrapper(queryClient?: QueryClient) {
  const qc =
    queryClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches tags without params', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [], total: 0 })

    const { result } = renderHook(() => useTags(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/tags')
  })

  it('includes category filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [], total: 0 })

    const { result } = renderHook(() => useTags({ category: 'genre' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('category=genre')
  })

  it('includes search filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [], total: 0 })

    const { result } = renderHook(() => useTags({ search: 'rock' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('search=rock')
  })

  it('includes pagination params', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [], total: 0 })

    const { result } = renderHook(() => useTags({ limit: 10, offset: 20 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=10')
    expect(url).toContain('offset=20')
  })
})

describe('useSearchTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('searches tags when query length >= 2', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [{ id: 1, name: 'rock' }] })

    const { result } = renderHook(() => useSearchTags('ro'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('q=ro')
  })

  it('does not search when query length < 2', () => {
    const { result } = renderHook(() => useSearchTags('r'), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not search with empty query', () => {
    const { result } = renderHook(() => useSearchTags(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('includes limit param', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [] })

    const { result } = renderHook(() => useSearchTags('rock', 5), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('limit=5')
  })
})

describe('useTag', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a tag by slug', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'rock', slug: 'rock' })

    const { result } = renderHook(() => useTag('rock'), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/tags/rock')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useTag('rock', { enabled: false }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useEntityTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches tags for an entity', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [{ tag_id: 1, name: 'rock' }] })

    const { result } = renderHook(() => useEntityTags('artist', 42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/entities/artist/42/tags')
  })

  it('does not fetch when entityId is 0', () => {
    const { result } = renderHook(() => useEntityTags('artist', 0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useAddTagToEntity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('adds a tag by ID', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAddTagToEntity(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ entityType: 'artist', entityId: 42, tag_id: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/entities/artist/42/tags',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ tag_id: 1, tag_name: undefined }),
      })
    )
  })

  it('adds a tag by name', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAddTagToEntity(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ entityType: 'venue', entityId: 10, tag_name: 'dive-bar' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/entities/venue/10/tags',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ tag_id: undefined, tag_name: 'dive-bar' }),
      })
    )
  })
})

describe('useRemoveTagFromEntity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('removes a tag from an entity', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useRemoveTagFromEntity(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artist', entityId: 42, tagId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/entities/artist/42/tags/1',
      expect.objectContaining({ method: 'DELETE' })
    )
  })
})

describe('useVoteOnTag (optimistic updates)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('votes on a tag-entity pair', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useVoteOnTag(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ tagId: 1, entityType: 'artist', entityId: 42, is_upvote: true })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/tags/1/entities/artist/42/votes',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ is_upvote: true }),
      })
    )
  })

  it('optimistically updates entity tags cache on upvote', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity }, mutations: { retry: false } },
    })

    queryClient.setQueryData(['tags', 'entityTags', 'artist', 42], {
      tags: [
        { tag_id: 1, name: 'rock', upvotes: 5, downvotes: 2, user_vote: 0 },
        { tag_id: 2, name: 'indie', upvotes: 3, downvotes: 1, user_vote: 0 },
      ],
    })

    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useVoteOnTag(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ tagId: 1, entityType: 'artist', entityId: 42, is_upvote: true })
    })

    const cached = queryClient.getQueryData<{ tags: Array<{ tag_id: number; upvotes: number; user_vote: number }> }>(
      ['tags', 'entityTags', 'artist', 42]
    )

    // The first tag should have been optimistically updated
    const tag = cached?.tags.find(t => t.tag_id === 1)
    expect(tag?.user_vote).toBe(1)
    expect(tag?.upvotes).toBe(6)

    // The second tag should be unchanged
    const otherTag = cached?.tags.find(t => t.tag_id === 2)
    expect(otherTag?.user_vote).toBe(0)
  })
})

describe('useRemoveTagVote', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('removes a vote with DELETE', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useRemoveTagVote(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ tagId: 1, entityType: 'artist', entityId: 42 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/tags/1/entities/artist/42/votes',
      expect.objectContaining({ method: 'DELETE' })
    )
  })
})
