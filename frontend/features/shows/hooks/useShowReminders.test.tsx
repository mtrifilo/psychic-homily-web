import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      SHOW_REMINDERS: '/auth/preferences/show-reminders',
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

import { useSetShowReminders } from './useShowReminders'


describe('useSetShowReminders', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('enables show reminders with PATCH', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, show_reminders: true })

    const { result } = renderHook(() => useSetShowReminders(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(true)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/show-reminders',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ enabled: true }),
      })
    )
  })

  it('disables show reminders with PATCH', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, show_reminders: false })

    const { result } = renderHook(() => useSetShowReminders(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(false)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/show-reminders',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ enabled: false }),
      })
    )
  })

  it('handles mutation errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetShowReminders(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(true)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeDefined()
  })
})
