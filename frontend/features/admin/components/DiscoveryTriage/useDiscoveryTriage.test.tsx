import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      LINK_SUGGESTIONS: {
        LIST: '/admin/link-suggestions',
        ACCEPT: (id: string | number) => `/admin/link-suggestions/${id}/accept`,
        REJECT: (id: string | number) => `/admin/link-suggestions/${id}/reject`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      linkSuggestions: (params?: Record<string, unknown>) => [
        'admin',
        'linkSuggestions',
        params,
      ],
    },
    artists: {
      all: ['artists'],
    },
  },
}))

import { useLinkSuggestions, useReviewLinkSuggestion } from './useDiscoveryTriage'

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

describe('useLinkSuggestions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('GETs the list endpoint with limit + offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ suggestions: [], total: 0 })
    const { queryClient } = setupClient()

    const { result } = renderHook(
      () => useLinkSuggestions({ limit: 25, offset: 0 }),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/link-suggestions?limit=25&offset=0'
    )
    expect(result.current.data).toEqual({ suggestions: [], total: 0 })
  })

  it('forwards a non-zero offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ suggestions: [], total: 30 })
    const { queryClient } = setupClient()

    renderHook(() => useLinkSuggestions({ limit: 25, offset: 25 }), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() =>
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/link-suggestions?limit=25&offset=25'
      )
    )
  })

  it('skips fetching when disabled', () => {
    const { queryClient } = setupClient()

    renderHook(() => useLinkSuggestions({ limit: 25, offset: 0, enabled: false }), {
      wrapper: createWrapperWithClient(queryClient),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})

describe('useReviewLinkSuggestion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('POSTs the accept endpoint and invalidates BOTH the queue and the artists prefix', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 1,
      artist_id: 42,
      status: 'accepted',
      reviewed_at: '2026-06-23T12:00:00Z',
      reviewed_by_user_id: 9,
    })
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useReviewLinkSuggestion(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ suggestionId: 1, verdict: 'accept' })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/link-suggestions/1/accept',
      expect.objectContaining({ method: 'POST' })
    )
    // Accept writes the artist's link, so the artists prefix must refetch
    // for the embed to render on the detail page.
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['admin', 'linkSuggestions'],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['artists'] })
  })

  it('POSTs the reject endpoint and invalidates ONLY the queue (not artists)', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 2,
      artist_id: 43,
      status: 'rejected',
    })
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useReviewLinkSuggestion(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ suggestionId: 2, verdict: 'reject' })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/link-suggestions/2/reject',
      expect.objectContaining({ method: 'POST' })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['admin', 'linkSuggestions'],
    })
    // Reject touches nothing on the artist — no broad artists invalidation.
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['artists'] })
  })

  it('sends NO body (verdict is in the path)', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 3, artist_id: 44, status: 'accepted' })
    const { queryClient } = setupClient()

    const { result } = renderHook(() => useReviewLinkSuggestion(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ suggestionId: 3, verdict: 'accept' })
    })

    const [, options] = mockApiRequest.mock.calls[0]
    expect(options).not.toHaveProperty('body')
  })

  it('surfaces a 409 error to the caller (does not swallow)', async () => {
    const conflict = Object.assign(new Error('conflict'), { status: 409 })
    mockApiRequest.mockRejectedValueOnce(conflict)
    const { queryClient } = setupClient()

    const { result } = renderHook(() => useReviewLinkSuggestion(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await expect(
      act(async () => {
        await result.current.mutateAsync({ suggestionId: 4, verdict: 'accept' })
      })
    ).rejects.toMatchObject({ status: 409 })
  })
})
