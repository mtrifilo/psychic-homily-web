import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Import hook after mocks are wired.
import { useCancelPendingEdit } from './useCancelPendingEdit'

describe('useCancelPendingEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('starts idle and exposes a mutate function', () => {
    mockApiRequest.mockResolvedValue({ success: true })

    const { result } = renderHook(() => useCancelPendingEdit(), {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    expect(result.current.isPending).toBe(false)
    expect(result.current.isSuccess).toBe(false)
    expect(typeof result.current.mutate).toBe('function')
  })

  it('issues a DELETE to the pending-edit endpoint for the given edit id', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useCancelPendingEdit(), {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    result.current.mutate(123)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/my/pending-edits/123',
      { method: 'DELETE' }
    )
    expect(result.current.data).toEqual({ success: true })
  })

  it('invalidates the my-pending-edits cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCancelPendingEdit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(7)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['my-pending-edits'],
    })
  })

  it('surfaces an error and does NOT invalidate the cache when the request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCancelPendingEdit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(99)

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(Error)
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})
