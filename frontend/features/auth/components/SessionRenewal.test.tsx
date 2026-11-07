import { describe, it, expect, vi, beforeEach } from 'vitest'
import { waitFor } from '@testing-library/react'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import type { UserProfile } from '@/features/auth/hooks/useAuth'
import { SessionRenewal } from './SessionRenewal'

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: vi.fn(),
}))

const mockApiRequest = vi.mocked(apiRequest)

function signedInProfile(overrides: Partial<UserProfile> = {}): UserProfile {
  return {
    success: true,
    message: 'Profile retrieved',
    user: {
      id: 1,
      email: 'viewer@example.com',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    ...overrides,
  }
}

function renderWithProfile(profile: UserProfile | undefined) {
  const queryClient = createTestQueryClient()
  if (profile) queryClient.setQueryData(queryKeys.auth.profile, profile)
  const result = renderWithProviders(<SessionRenewal />, { queryClient })
  return { ...result, queryClient }
}

describe('SessionRenewal', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('posts to /auth/refresh once when the seeded profile says the session is due', async () => {
    mockApiRequest.mockResolvedValue({ success: true, message: 'Token refreshed' })
    const { queryClient, rerender } = renderWithProfile(
      signedInProfile({ session_renewal_due: true })
    )

    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))
    expect(mockApiRequest).toHaveBeenCalledWith(
      API_ENDPOINTS.AUTH.REFRESH,
      expect.objectContaining({ method: 'POST', credentials: 'include' })
    )

    // Success marks the cached profile as no longer due, so a re-render asks nothing more.
    await waitFor(() =>
      expect(
        queryClient.getQueryData<UserProfile>(queryKeys.auth.profile)?.session_renewal_due
      ).toBe(false)
    )
    rerender(<SessionRenewal />)
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })

  it('does nothing when the session is not due', async () => {
    renderWithProfile(signedInProfile())
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('leaves the signed-in profile untouched when the renewal fails', async () => {
    mockApiRequest.mockRejectedValue(new Error('network down'))
    const profile = signedInProfile({ session_renewal_due: true })
    const { queryClient } = renderWithProfile(profile)

    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(queryClient.getQueryData<UserProfile>(queryKeys.auth.profile)).toEqual(profile)
  })

  it('asks only once per page load even if the flag stays set', async () => {
    mockApiRequest.mockResolvedValue({ success: true, message: 'Token refreshed' })
    const { queryClient, rerender } = renderWithProfile(
      signedInProfile({ session_renewal_due: true })
    )
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))

    // A refetch that still says due (for example a stale read) must not re-arm it.
    queryClient.setQueryData(queryKeys.auth.profile, signedInProfile({ session_renewal_due: true }))
    rerender(<SessionRenewal />)
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })
})
