import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    TAGS: {
      GET: (idOrSlug: string | number) => `/tags/${idOrSlug}`,
      ALIASES: (idOrSlug: string | number) => `/tags/${idOrSlug}/aliases`,
      DELETE_ALIAS: (tagId: number, aliasId: number) =>
        `/tags/${tagId}/aliases/${aliasId}`,
      ADMIN_MERGE: (sourceId: number) => `/admin/tags/${sourceId}/merge`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module. The hooks call queryClient.invalidateQueries
// directly with these keys, so we mirror the real key shapes and assert
// against them via a spy on the live QueryClient below.
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    tags: {
      all: ['tags'],
      detail: (id: string | number) => ['tags', 'detail', String(id)],
      aliases: (tagId: number) => ['tags', 'aliases', tagId],
    },
  },
}))

// Import hooks after mocks are set up
import {
  useDeleteTag,
  useMergeTags,
  useCreateAlias,
  useDeleteAlias,
} from './useAdminTags'

/** Build a retry-free QueryClient plus a spy on its invalidateQueries. */
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

describe('useDeleteTag', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('DELETEs the tag and invalidates the tag list on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useDeleteTag(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync(5)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/tags/5', { method: 'DELETE' })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['tags'] })
  })

  it('surfaces the error and skips invalidation when delete is blocked', async () => {
    const error = new Error('tag is in use and cannot be deleted')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useDeleteTag(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync(5)
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe(
      'tag is in use and cannot be deleted'
    )
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})

describe('useMergeTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs source→target and invalidates tag lists + aliases on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ merged_count: 7 })
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useMergeTags(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ sourceId: 3, targetId: 8 })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/admin/tags/3/merge', {
      method: 'POST',
      body: JSON.stringify({ target_id: 8 }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['tags'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['tags', 'aliases'] })
  })

  it('surfaces a merge error and skips invalidation', async () => {
    const error = new Error('cannot merge a tag into itself')
    Object.assign(error, { status: 400 })
    mockApiRequest.mockRejectedValueOnce(error)
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useMergeTags(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({ sourceId: 3, targetId: 3 })
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe(
      'cannot merge a tag into itself'
    )
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})

describe('useCreateAlias', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs the alias and invalidates that tag\'s aliases + detail', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useCreateAlias(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ tagId: 5, alias: 'hardcore-punk' })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/tags/5/aliases', {
      method: 'POST',
      body: JSON.stringify({ alias: 'hardcore-punk' }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['tags', 'aliases', 5],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['tags', 'detail', '5'],
    })
  })

  it('surfaces a duplicate-alias error and skips invalidation', async () => {
    const error = new Error('alias already exists')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useCreateAlias(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({ tagId: 5, alias: 'punk' })
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('alias already exists')
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})

describe('useDeleteAlias', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('DELETEs the alias and invalidates that tag\'s aliases + detail', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useDeleteAlias(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ tagId: 5, aliasId: 99 })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/tags/5/aliases/99', {
      method: 'DELETE',
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['tags', 'aliases', 5],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['tags', 'detail', '5'],
    })
  })
})
