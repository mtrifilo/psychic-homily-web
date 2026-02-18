import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderWithProviders, waitFor } from '@/test/utils'
import { act } from '@testing-library/react'
import MagicLinkPage from './page'

const mockPush = vi.fn()
const mockSetUser = vi.fn()
const mockMutate = vi.fn()
const mockUseVerifyMagicLink = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => new URLSearchParams('token=magic-token'),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    setUser: mockSetUser,
  }),
}))

vi.mock('@/lib/hooks/useAuth', () => ({
  useVerifyMagicLink: () => mockUseVerifyMagicLink(),
}))

describe('MagicLinkPage', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockSetUser.mockReset()
    mockMutate.mockReset()
    mockUseVerifyMagicLink.mockReset()

    mockUseVerifyMagicLink.mockImplementation(() => ({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    }))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('verifies magic link only once per token across re-renders', async () => {
    const { rerender } = renderWithProviders(<MagicLinkPage />)

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledTimes(1)
      expect(mockMutate).toHaveBeenCalledWith('magic-token', expect.any(Object))
    })

    rerender(<MagicLinkPage />)

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledTimes(1)
    })
  })

  it('redirects after successful verification', async () => {
    vi.useFakeTimers()
    mockMutate.mockImplementation(
      (_token: string, options?: { onSuccess?: (data: unknown) => void }) => {
        options?.onSuccess?.({
          user: {
            id: 1,
            email: 'magic@example.com',
            first_name: 'Magic',
            last_name: 'User',
            is_admin: false,
          },
        })
      }
    )

    renderWithProviders(<MagicLinkPage />)

    await act(async () => {})
    expect(mockMutate).toHaveBeenCalledTimes(1)

    await act(async () => {
      vi.advanceTimersByTime(1500)
    })

    expect(mockPush).toHaveBeenCalledWith('/')
  })

  it('cleans up scheduled redirect on unmount', async () => {
    vi.useFakeTimers()
    mockMutate.mockImplementation(
      (_token: string, options?: { onSuccess?: (data: unknown) => void }) => {
        options?.onSuccess?.({
          user: {
            id: 1,
            email: 'magic@example.com',
            first_name: 'Magic',
            last_name: 'User',
            is_admin: false,
          },
        })
      }
    )

    const { unmount } = renderWithProviders(<MagicLinkPage />)

    await act(async () => {})
    expect(mockMutate).toHaveBeenCalledTimes(1)

    unmount()
    await act(async () => {
      vi.advanceTimersByTime(1500)
    })

    expect(mockPush).not.toHaveBeenCalled()
  })
})
