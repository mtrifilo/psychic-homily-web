import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
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

  it('does not claim expiry when the URL carries no token at all', () => {
    mockToken = null

    renderWithProviders(<VerifyEmailPage />)

    expect(
      screen.getByRole('heading', { name: 'That link is not valid.' })
    ).toBeInTheDocument()
    expect(mockApiRequest).not.toHaveBeenCalled()
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
