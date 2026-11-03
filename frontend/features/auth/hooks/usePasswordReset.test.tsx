import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapperWithClient, createTestQueryClient } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      PASSWORD_RESET_REQUEST: '/auth/password-reset/request',
      PASSWORD_RESET_SEND_OWN: '/auth/password-reset/send-own',
      PASSWORD_RESET_CHECK: '/auth/password-reset/check',
      PASSWORD_RESET_COMPLETE: '/auth/password-reset/complete',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

const {
  useRequestPasswordReset,
  useSendOwnPasswordReset,
  useCheckPasswordReset,
  useCompletePasswordReset,
} = await import('./useAuth')

function setup() {
  const queryClient = createTestQueryClient()
  const refetch = vi.spyOn(queryClient, 'refetchQueries').mockResolvedValue()
  return { wrapper: createWrapperWithClient(queryClient), refetch }
}

describe('password reset hooks', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('asks for a reset link for the given address', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'sent' })
    const { result } = renderHook(() => useRequestPasswordReset(), { wrapper: setup().wrapper })

    await act(async () => {
      await result.current.mutateAsync('owner@example.com')
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/password-reset/request',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ email: 'owner@example.com' }) })
    )
  })

  it('sends the signed-in account its own link with no body', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
    const { result } = renderHook(() => useSendOwnPasswordReset(), { wrapper: setup().wrapper })

    await act(async () => {
      await result.current.mutateAsync()
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/password-reset/send-own',
      expect.objectContaining({ method: 'POST', credentials: 'include' })
    )
    expect(mockApiRequest.mock.calls[0][1]).not.toHaveProperty('body')
  })

  it('checks nothing without a link', () => {
    renderHook(() => useCheckPasswordReset(null), { wrapper: setup().wrapper })

    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('raises a refused check as an AuthError carrying its code and a sub-500 status', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: false, message: 'gone', error_code: 'INVALID_TOKEN' })
    const { result } = renderHook(() => useCheckPasswordReset('used-link'), { wrapper: setup().wrapper })

    await waitFor(() => expect(result.current.isError).toBe(true))
    const error = result.current.error as AuthError
    expect(error).toBeInstanceOf(AuthError)
    expect(error.code).toBe(AuthErrorCode.INVALID_TOKEN)
    expect(error.status).toBe(400)
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/password-reset/check',
      expect.objectContaining({ body: JSON.stringify({ token: 'used-link' }) })
    )
  })

  it('sets the password and brings the profile up to date with the new session', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Password reset' })
    const { wrapper, refetch } = setup()
    const { result } = renderHook(() => useCompletePasswordReset(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ token: 'reset-link', newPassword: 'owners-new-password-1!' })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/password-reset/complete',
      expect.objectContaining({
        body: JSON.stringify({ token: 'reset-link', new_password: 'owners-new-password-1!' }),
      })
    )
    expect(refetch).toHaveBeenCalledWith({ queryKey: ['auth', 'profile'] })
  })

  it('refreshes nothing when the set is refused', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: false, message: 'weak', error_code: 'VALIDATION_FAILED' })
    const { wrapper, refetch } = setup()
    const { result } = renderHook(() => useCompletePasswordReset(), { wrapper })

    await act(async () => {
      result.current.mutate({ token: 'reset-link', newPassword: 'weak' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as AuthError).code).toBe(AuthErrorCode.VALIDATION_FAILED)
    expect(refetch).not.toHaveBeenCalled()
  })
})
