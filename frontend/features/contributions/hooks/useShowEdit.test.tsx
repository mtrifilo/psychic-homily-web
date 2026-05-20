import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Import hook after mocks are wired.
import { useShowEdit } from './useShowEdit'

describe('useShowEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('starts idle and exposes a mutate function', () => {
    mockApiRequest.mockResolvedValue(undefined)

    const { result } = renderHook(() => useShowEdit(), {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    expect(result.current.isPending).toBe(false)
    expect(result.current.isSuccess).toBe(false)
    expect(typeof result.current.mutate).toBe('function')
  })

  it('PUTs the show-update body and resolves with a synthetic applied:true result', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useShowEdit(), {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    result.current.mutate({
      entityId: 55,
      changes: [
        { field: 'title', old_value: 'Old', new_value: 'New Title' },
        { field: 'age_requirement', old_value: '18+', new_value: '21+' },
      ],
      summary: 'fix title + age',
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/shows/55',
      {
        method: 'PUT',
        body: JSON.stringify({
          title: 'New Title',
          age_requirement: '21+',
          summary: 'fix title + age',
        }),
      }
    )

    // Show edits always apply directly — the hook fabricates a
    // SuggestEditResponse so the drawer's UI logic doesn't need to fork.
    expect(result.current.data).toEqual({
      applied: true,
      message: 'Changes saved',
    })
  })

  it('translates a blanked (null) field to an empty string in the body', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useShowEdit(), {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    result.current.mutate({
      entityId: 12,
      changes: [{ field: 'description', old_value: 'something', new_value: null }],
      summary: 'clear description',
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/shows/12',
      {
        method: 'PUT',
        body: JSON.stringify({
          description: '',
          summary: 'clear description',
        }),
      }
    )
  })

  it('invalidates the shows cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useShowEdit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate({
      entityId: 1,
      changes: [{ field: 'title', old_value: 'a', new_value: 'b' }],
      summary: 'rename',
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['shows'] })
  })

  it('surfaces an error and does NOT invalidate the cache when the PUT fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useShowEdit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate({
      entityId: 9,
      changes: [{ field: 'title', old_value: 'a', new_value: 'b' }],
      summary: 'rename',
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(Error)
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})
