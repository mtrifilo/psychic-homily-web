import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      TIER_EDIT_NOTIFICATIONS: '/auth/preferences/tier-edit-notifications',
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

import { useSetTierEditNotificationPreference } from './useTierEditNotificationPreference'

describe('useSetTierEditNotificationPreference', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('opts out of tier emails with PATCH and only the tier field', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      notify_on_tier_notifications: false,
      notify_on_edit_notifications: true,
    })

    const { result } = renderHook(
      () => useSetTierEditNotificationPreference(),
      { wrapper: createWrapper() }
    )

    await act(async () => {
      result.current.mutate({ notify_on_tier_notifications: false })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/tier-edit-notifications',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ notify_on_tier_notifications: false }),
      })
    )
  })

  it('opts out of edit emails with PATCH and only the edit field', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      notify_on_tier_notifications: true,
      notify_on_edit_notifications: false,
    })

    const { result } = renderHook(
      () => useSetTierEditNotificationPreference(),
      { wrapper: createWrapper() }
    )

    await act(async () => {
      result.current.mutate({ notify_on_edit_notifications: false })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/tier-edit-notifications',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ notify_on_edit_notifications: false }),
      })
    )
  })

  it('re-enables tier emails with PATCH and only the tier field', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      notify_on_tier_notifications: true,
      notify_on_edit_notifications: true,
    })

    const { result } = renderHook(
      () => useSetTierEditNotificationPreference(),
      { wrapper: createWrapper() }
    )

    await act(async () => {
      result.current.mutate({ notify_on_tier_notifications: true })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/tier-edit-notifications',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ notify_on_tier_notifications: true }),
      })
    )
  })

  it('surfaces mutation errors so the UI can show its error block', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => useSetTierEditNotificationPreference(),
      { wrapper: createWrapper() }
    )

    await act(async () => {
      result.current.mutate({ notify_on_tier_notifications: false })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeDefined()
  })
})
