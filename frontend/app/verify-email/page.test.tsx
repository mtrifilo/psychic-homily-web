import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { apiRequest } from '@/lib/api'
import VerifyEmailPage from './page'

// --- Mocks ---
//
// The REAL verification hooks run here, against a mocked apiRequest, so the
// landing states are consequences of the request rather than presets.

let mockToken: string | null = 'verify-token'
vi.mock('next/navigation', () => ({
  useSearchParams: () =>
    new URLSearchParams(mockToken ? `token=${mockToken}` : ''),
}))

let mockIsAuthenticated = true
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: mockIsAuthenticated }),
}))

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: vi.fn(),
}))

const mockApiRequest = vi.mocked(apiRequest)

describe('VerifyEmailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockToken = 'verify-token'
    mockIsAuthenticated = true
  })

  it('confirms the token only once across re-renders', async () => {
    mockApiRequest.mockResolvedValue({ success: true })

    const { rerender } = renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    })
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringContaining('/auth/verify-email/confirm'),
      expect.objectContaining({ body: JSON.stringify({ token: 'verify-token' }) })
    )

    rerender(<VerifyEmailPage />)

    await waitFor(() => {
      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    })
  })

  it('lands a confirmed email on the radar block with ALERTS called out', async () => {
    mockApiRequest.mockResolvedValue({ success: true })

    renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: 'Welcome to the index.' })
      ).toBeInTheDocument()
    })

    expect(
      screen.getByText(/Email confirmed · Submissions open/i)
    ).toBeInTheDocument()
    for (const rung of ['SAVE', 'FOLLOW', 'ALERTS', 'SUBMIT']) {
      expect(screen.getByText(rung)).toBeInTheDocument()
    }
    expect(
      screen.getByText('choose what gets emailed to you in Settings')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Browse upcoming shows' })
    ).toHaveAttribute('href', '/shows')
    expect(screen.getByRole('link', { name: 'Explore artists' })).toHaveAttribute(
      'href',
      '/artists'
    )
  })

  it('shows the expired card with a fresh-link action when confirmation fails', async () => {
    mockApiRequest.mockResolvedValue({
      success: false,
      message: 'token expired',
    })

    renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: 'That link has expired.' })
      ).toBeInTheDocument()
    })
    expect(
      screen.getByText(/Contributor record · Link expired/i)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Send a fresh link/i })
    ).toBeInTheDocument()
    // The backend's raw failure text never reaches the reader.
    expect(screen.queryByText(/token expired/)).not.toBeInTheDocument()
  })

  // A dropped connection or a 5xx says nothing about the link. Calling it
  // expired sends the reader to fetch a replacement that fails the same way.
  it('does not call the link expired when the check itself failed', async () => {
    mockApiRequest.mockRejectedValue(
      Object.assign(new Error('Network request failed'), { status: 503 })
    )

    renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: 'We could not check that link.' })
      ).toBeInTheDocument()
    })
    expect(
      screen.queryByRole('heading', { name: 'That link has expired.' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try again' })
    ).toBeInTheDocument()
  })

  it('retries the same token from the check-failed card', async () => {
    mockApiRequest.mockRejectedValue(
      Object.assign(new Error('Network request failed'), { status: 503 })
    )

    renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Try again' })
      ).toBeInTheDocument()
    })
    const callsBeforeRetry = mockApiRequest.mock.calls.length

    await userEvent.click(screen.getByRole('button', { name: 'Try again' }))

    await waitFor(() => {
      expect(mockApiRequest.mock.calls.length).toBeGreaterThan(callsBeforeRetry)
    })
    expect(mockApiRequest).toHaveBeenLastCalledWith(
      expect.stringContaining('/auth/verify-email/confirm'),
      expect.objectContaining({
        body: JSON.stringify({ token: 'verify-token' }),
      })
    )
  })

  it('does not claim expiry when the URL carries no token at all', () => {
    mockToken = null

    renderWithProviders(<VerifyEmailPage />)

    expect(
      screen.getByRole('heading', { name: 'That link is not valid.' })
    ).toBeInTheDocument()
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  // The dead-link card carries its own copy of the resend handler, so the
  // throttle and session branches are pinned here too and not only on the gate.
  it('renders a throttled fresh-link request as a cooldown', async () => {
    mockApiRequest.mockImplementation(async (path: string) => {
      if (path.includes('/auth/verify-email/send')) {
        throw Object.assign(new Error('Rate limit exceeded.'), {
          status: 429,
          retryAfter: 25,
        })
      }
      return { success: false, message: 'token expired' }
    })

    renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Send a fresh link/i })
      ).toBeInTheDocument()
    })

    await userEvent.click(
      screen.getByRole('button', { name: /Send a fresh link/i })
    )

    await waitFor(() => {
      expect(screen.getByText('Resend available in 25s')).toBeInTheDocument()
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Send a fresh link/i })
    ).toBeDisabled()
  })

  it('offers sign-in when the session died while the card sat open', async () => {
    mockApiRequest.mockImplementation(async (path: string) => {
      if (path.includes('/auth/verify-email/send')) {
        throw Object.assign(new Error('unauthorized'), { status: 401 })
      }
      return { success: false, message: 'token expired' }
    })

    renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Send a fresh link/i })
      ).toBeInTheDocument()
    })

    await userEvent.click(
      screen.getByRole('button', { name: /Send a fresh link/i })
    )

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Your session has expired.'
      )
    })
    expect(
      screen.getByRole('link', { name: /Sign in to send a fresh link/i })
    ).toHaveAttribute('href', '/auth?returnTo=%2Fprofile%3Ftab%3Dsettings')
    expect(
      screen.queryByText(/We could not send that email just now/)
    ).not.toBeInTheDocument()
  })

  it('routes a signed-out visitor through sign-in instead of a dead resend', () => {
    mockToken = null
    mockIsAuthenticated = false

    renderWithProviders(<VerifyEmailPage />)

    expect(
      screen.getByRole('link', { name: /Sign in to send a fresh link/i })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Send a fresh link/i })
    ).not.toBeInTheDocument()
  })
})
