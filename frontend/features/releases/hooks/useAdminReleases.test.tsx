import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateReleases = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/features/releases/api', () => ({
  releaseEndpoints: {
    CREATE: '/releases',
    UPDATE: (id: string | number) => `/releases/${id}`,
    DELETE: (id: string | number) => `/releases/${id}`,
    ADD_LINK: (id: string | number) => `/releases/${id}/links`,
    REMOVE_LINK: (id: string | number, linkId: string | number) =>
      `/releases/${id}/links/${linkId}`,
  },
}))

// createInvalidateQueries(queryClient).releases() is the cache-bust path all
// five admin mutations share on success.
vi.mock('@/lib/queryClient', () => ({
  createInvalidateQueries: () => ({
    releases: mockInvalidateReleases,
  }),
}))

import {
  useCreateRelease,
  useUpdateRelease,
  useDeleteRelease,
  useAddReleaseLink,
  useRemoveReleaseLink,
} from './useAdminReleases'

beforeEach(() => {
  vi.clearAllMocks()
  mockApiRequest.mockReset()
  mockInvalidateReleases.mockReset()
})

describe('useCreateRelease', () => {
  it('POSTs the release payload to the create endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'New LP' })

    const { result } = renderHook(() => useCreateRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ title: 'New LP', release_type: 'lp' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/releases', {
      method: 'POST',
      body: JSON.stringify({ title: 'New LP', release_type: 'lp' }),
    })
  })

  it('invalidates the releases cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'New LP' })

    const { result } = renderHook(() => useCreateRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ title: 'New LP' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateReleases).toHaveBeenCalledTimes(1)
  })

  it('surfaces errors and does not invalidate the cache', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Validation failed'))

    const { result } = renderHook(() => useCreateRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ title: '' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Validation failed')
    expect(mockInvalidateReleases).not.toHaveBeenCalled()
  })

  it('reports a pending state while in flight', async () => {
    let resolve!: (value: unknown) => void
    mockApiRequest.mockReturnValueOnce(
      new Promise(r => {
        resolve = r
      })
    )

    const { result } = renderHook(() => useCreateRelease(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate({ title: 'Pending' })
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolve({ id: 2, title: 'Pending' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })
})

describe('useUpdateRelease', () => {
  it('PUTs the update payload to the id-scoped endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 7, title: 'Edited' })

    const { result } = renderHook(() => useUpdateRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        releaseId: 7,
        data: { title: 'Edited', release_year: null },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/releases/7', {
      method: 'PUT',
      body: JSON.stringify({ title: 'Edited', release_year: null }),
    })
  })

  it('invalidates the releases cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 7, title: 'Edited' })

    const { result } = renderHook(() => useUpdateRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ releaseId: 7, data: { title: 'Edited' } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateReleases).toHaveBeenCalledTimes(1)
  })

  it('does not invalidate the cache on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Not found'))

    const { result } = renderHook(() => useUpdateRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ releaseId: 999, data: { title: 'X' } })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(mockInvalidateReleases).not.toHaveBeenCalled()
  })
})

describe('useDeleteRelease', () => {
  it('DELETEs the id-scoped endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/releases/42', {
      method: 'DELETE',
    })
  })

  it('invalidates the releases cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateReleases).toHaveBeenCalledTimes(1)
  })

  it('propagates delete errors without invalidating', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Conflict'))

    const { result } = renderHook(() => useDeleteRelease(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(mockInvalidateReleases).not.toHaveBeenCalled()
  })
})

describe('useAddReleaseLink', () => {
  it('POSTs platform + url to the links endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 5,
      platform: 'bandcamp',
      url: 'https://x.bandcamp.com',
    })

    const { result } = renderHook(() => useAddReleaseLink(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        releaseId: 7,
        platform: 'bandcamp',
        url: 'https://x.bandcamp.com',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/releases/7/links', {
      method: 'POST',
      body: JSON.stringify({
        platform: 'bandcamp',
        url: 'https://x.bandcamp.com',
      }),
    })
  })

  it('invalidates the releases cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 5 })

    const { result } = renderHook(() => useAddReleaseLink(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        releaseId: 7,
        platform: 'spotify',
        url: 'https://s',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateReleases).toHaveBeenCalledTimes(1)
  })
})

describe('useRemoveReleaseLink', () => {
  it('DELETEs the nested link endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useRemoveReleaseLink(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ releaseId: 7, linkId: 99 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/releases/7/links/99', {
      method: 'DELETE',
    })
  })

  it('invalidates the releases cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useRemoveReleaseLink(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ releaseId: 7, linkId: 99 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateReleases).toHaveBeenCalledTimes(1)
  })

  it('does not invalidate the cache on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useRemoveReleaseLink(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ releaseId: 7, linkId: 99 })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(mockInvalidateReleases).not.toHaveBeenCalled()
  })
})
