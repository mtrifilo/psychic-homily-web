import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

import { API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useScenes } from './useScenes'

/**
 * `app/scenes/page.tsx` server-renders the scene grid by fetching
 * `API_ENDPOINTS.SCENES.LIST` and seeding `queryKeys.scenes.list` (PSY-1624).
 *
 * `useScenes` takes no arguments today, which is exactly why this is worth
 * pinning: the moment it gains a parameter that reaches its key or URL, the
 * page's seed stops being the entry it reads, the hook falls back to fetching,
 * and /scenes silently returns to shipping an empty shell. Nothing else fails.
 */
describe('scenes first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('requests SCENES.LIST and keys on queryKeys.scenes.list', async () => {
    mockApiRequest.mockResolvedValueOnce({ scenes: [], count: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useScenes(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(API_ENDPOINTS.SCENES.LIST, {
      method: 'GET',
    })

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(queryKeys.scenes.list))
  })
})
