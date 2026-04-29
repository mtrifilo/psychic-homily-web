import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      COLLECTION_DIGEST: '/auth/preferences/collection-digest',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    auth: {
      profile: ['auth', 'profile'],
    },
  },
}))

import { useSetCollectionDigestPreference } from './useCollectionDigestPreference'

describe('useSetCollectionDigestPreference', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('enables the digest with PATCH and the right body', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      notify_on_collection_digest: true,
    })

    const { result } = renderHook(() => useSetCollectionDigestPreference(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(true)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/collection-digest',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ enabled: true }),
      })
    )
  })

  it('disables the digest with PATCH and the right body', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      notify_on_collection_digest: false,
    })

    const { result } = renderHook(() => useSetCollectionDigestPreference(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(false)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/collection-digest',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ enabled: false }),
      })
    )
  })

  it('surfaces mutation errors so the UI can show its error block', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetCollectionDigestPreference(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(true)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeDefined()
  })
})
