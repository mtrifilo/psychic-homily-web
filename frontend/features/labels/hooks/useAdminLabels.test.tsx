import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateLabels = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  createInvalidateQueries: () => ({
    labels: mockInvalidateLabels,
  }),
}))

vi.mock('@/features/labels/api', () => ({
  labelEndpoints: {
    CREATE: '/labels',
    UPDATE: (labelId: number) => `/labels/${labelId}`,
    DELETE: (labelId: number) => `/labels/${labelId}`,
  },
}))

import { useCreateLabel, useUpdateLabel, useDeleteLabel } from './useAdminLabels'

describe('useCreateLabel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateLabels.mockReset()
  })

  it('POSTs the new label payload to the create endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 10, name: 'Sub Pop' })

    const { result } = renderHook(() => useCreateLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: 'Sub Pop', city: 'Seattle' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels', {
      method: 'POST',
      body: JSON.stringify({ name: 'Sub Pop', city: 'Seattle' }),
    })
  })

  it('invalidates the labels cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 10, name: 'Sub Pop' })

    const { result } = renderHook(() => useCreateLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: 'Sub Pop' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateLabels).toHaveBeenCalled()
  })

  it('surfaces the error and skips invalidation on failure', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Name already taken'))

    const { result } = renderHook(() => useCreateLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: 'Dupe' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Name already taken')
    expect(mockInvalidateLabels).not.toHaveBeenCalled()
  })

  it('reports the pending state while the request is in flight', async () => {
    mockApiRequest.mockReturnValue(new Promise(() => {}))

    const { result } = renderHook(() => useCreateLabel(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(false)
    act(() => {
      result.current.mutate({ name: 'Slow Pop' })
    })
    await waitFor(() => expect(result.current.isPending).toBe(true))
  })
})

describe('useUpdateLabel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateLabels.mockReset()
  })

  it('PUTs the data payload to the label-id endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 5, name: 'Renamed' })

    const { result } = renderHook(() => useUpdateLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ labelId: 5, data: { name: 'Renamed' } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels/5', {
      method: 'PUT',
      body: JSON.stringify({ name: 'Renamed' }),
    })
  })

  it('invalidates the labels cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 5, name: 'Renamed' })

    const { result } = renderHook(() => useUpdateLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ labelId: 5, data: { name: 'Renamed' } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateLabels).toHaveBeenCalled()
  })

  it('surfaces an update error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Label not found'))

    const { result } = renderHook(() => useUpdateLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ labelId: 999, data: { name: 'Ghost' } })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Label not found')
  })
})

describe('useDeleteLabel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateLabels.mockReset()
  })

  it('DELETEs against the label-id endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(8)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels/8', {
      method: 'DELETE',
    })
  })

  it('invalidates the labels cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(8)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateLabels).toHaveBeenCalled()
  })

  it('surfaces a delete error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Delete blocked'))

    const { result } = renderHook(() => useDeleteLabel(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(8)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Delete blocked')
  })
})
