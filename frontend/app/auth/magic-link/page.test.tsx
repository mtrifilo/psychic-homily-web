import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderWithProviders, waitFor, screen } from '@/test/utils'
import { act } from '@testing-library/react'
import { AuthError } from '@/lib/errors'
import type { ApiError } from '@/lib/api'
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

vi.mock('@/features/auth', () => ({
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

  it('renders "Link Expired" for a body-encoded INVALID_TOKEN error', async () => {
    // PSY-875/PSY-881: the invalid/expired-token path returns HTTP 200 +
    // { success: false, error_code: 'INVALID_TOKEN' }, which the hook
    // re-throws as an AuthError with status 401. A new link is the correct
    // remedy, so this stays on the "Link Expired" pane.
    const bodyError = new AuthError(
      'This magic link has expired or is invalid.',
      'TOKEN_INVALID',
      { status: 401 }
    )
    mockUseVerifyMagicLink.mockImplementation(() => ({
      mutate: mockMutate,
      reset: vi.fn(),
      isPending: false,
      isError: true,
      isSuccess: false,
      error: bodyError,
    }))

    renderWithProviders(<MagicLinkPage />)

    await waitFor(() => {
      expect(screen.getByText('Link Expired')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('button', { name: /back to sign in/i })
    ).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument()
  })

  it('renders the retry pane (not "Link Expired") for a 5xx error', async () => {
    // PSY-875 flipped the JWT-mint failure from a silent HTTP 200 to a real
    // 5xx (ApiError with status >= 500). Requesting a new link can't fix a
    // backend outage, so this renders the server-error + retry pane.
    const serverError: ApiError = Object.assign(
      new Error('HTTP 500: Internal Server Error'),
      { status: 500 }
    )
    const mockReset = vi.fn()
    mockUseVerifyMagicLink.mockImplementation(() => ({
      mutate: mockMutate,
      reset: mockReset,
      isPending: false,
      isError: true,
      isSuccess: false,
      error: serverError,
    }))

    renderWithProviders(<MagicLinkPage />)

    await waitFor(() => {
      expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    })
    expect(screen.queryByText('Link Expired')).not.toBeInTheDocument()

    const retryButton = screen.getByRole('button', { name: /try again/i })
    expect(retryButton).toBeInTheDocument()
    // There is no request-new-link CTA on the server-error pane.
    expect(
      screen.queryByRole('button', { name: /back to sign in/i })
    ).not.toBeInTheDocument()

    // Retry resets the mutation so the same token is re-verified.
    await act(async () => {
      retryButton.click()
    })
    expect(mockReset).toHaveBeenCalledTimes(1)
  })

  it('renders the retry pane for a status-less network failure', async () => {
    // A network outage re-throws the raw fetch error (a TypeError with no
    // `status`). That is also a "try again" situation, not "link expired".
    const networkError = new TypeError('Failed to fetch')
    mockUseVerifyMagicLink.mockImplementation(() => ({
      mutate: mockMutate,
      reset: vi.fn(),
      isPending: false,
      isError: true,
      isSuccess: false,
      error: networkError,
    }))

    renderWithProviders(<MagicLinkPage />)

    await waitFor(() => {
      expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    })
    expect(screen.queryByText('Link Expired')).not.toBeInTheDocument()
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
