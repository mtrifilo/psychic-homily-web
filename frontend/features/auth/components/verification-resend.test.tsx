import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as Sentry from '@sentry/nextjs'
import { renderWithProviders } from '@/test/utils'
import { apiRequest } from '@/lib/api'
import {
  VerificationResend,
  VerificationResendAlerts,
  VerificationResendButton,
  VerificationResendStatus,
} from './verification-resend'

// --- Mocks ---
//
// The REAL useSendVerificationEmail hook and cooldown run here, against a mocked
// apiRequest, so every state is a consequence of the click. @sentry/nextjs is
// mocked globally in test/setup.ts.

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: vi.fn(),
}))

const mockApiRequest = vi.mocked(apiRequest)

const resendButton = () => screen.getByRole('button', { name: 'Send it again' })

function renderControl(density?: 'default' | 'compact') {
  return renderWithProviders(
    <VerificationResend service="test_surface" signInHref="/auth?returnTo=%2F">
      <VerificationResendButton>Send it again</VerificationResendButton>
      <VerificationResendStatus density={density} />
      <VerificationResendAlerts />
    </VerificationResend>
  )
}

describe('VerificationResend', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('refuses to render its parts outside a provider', () => {
    // The thrown error is the point, but React logs the boundary-less throw.
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)

    expect(() =>
      renderWithProviders(
        <VerificationResendButton>Send it again</VerificationResendButton>
      )
    ).toThrow(/must be rendered inside <VerificationResend>/)

    consoleError.mockRestore()
  })

  it('sends, confirms, and parks the control for the standard cooldown', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(
        screen.getByText('Sent · Check your inbox · Resend available in 60s')
      ).toBeInTheDocument()
    })
    expect(resendButton()).toBeDisabled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    // The live region carries the state once; the ticking line is hidden from
    // it so it does not announce sixty times over one cooldown.
    expect(screen.getByRole('status')).toHaveTextContent(
      'Verification email sent. Check your inbox.'
    )
    expect(screen.getByRole('status')).not.toHaveTextContent(/\d/)
    expect(screen.getByText(/Resend available in 60s/)).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })

  // ONE 429 voice: a throttle is a wait, never an error, and never leaks the
  // backend's own wording.
  it('renders a throttle whose Retry-After was readable as an exact countdown', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('Rate limit exceeded.'), {
        status: 429,
        retryAfter: 25,
      })
    )
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByText('Resend available in 25s')).toBeInTheDocument()
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText(/Rate limit exceeded/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Check your inbox/)).not.toBeInTheDocument()
    expect(resendButton()).toBeDisabled()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Resend is not available yet. Please wait a moment.'
    )
  })

  // The production path. CORS does not expose Retry-After (PSY-1924), so the
  // app parks for its own standard cooldown but must not dress that assumption
  // up as a second count it read off the server.
  it('states an unreadable-Retry-After throttle approximately, with no second count', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('Rate limit exceeded.'), { status: 429 })
    )
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(
        screen.getByText('Resend available in about a minute')
      ).toBeInTheDocument()
    })
    expect(screen.queryByText(/\d/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    // Approximate copy, but the same real wait: the button is parked either way.
    expect(resendButton()).toBeDisabled()
  })

  it('gives a dead session a way back rather than a generic failure', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('unauthorized'), { status: 401 })
    )
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Your session has expired.'
      )
    })
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      '/auth?returnTo=%2F'
    )
    expect(
      screen.queryByText(/We could not send that email just now/)
    ).not.toBeInTheDocument()
    // An expiring cookie is an ordinary event, not something to page on-call for.
    expect(Sentry.captureException).not.toHaveBeenCalled()
  })

  it('reports a genuine failure to Sentry under the surface it happened on', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('Email service is not configured'), {
        status: 500,
      })
    )
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'We could not send that email just now. Please try again in a moment.'
      )
    })
    expect(
      screen.queryByText(/Email service is not configured/)
    ).not.toBeInTheDocument()
    expect(Sentry.captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({
        tags: { service: 'test_surface', error_type: 'verification_email' },
      })
    )
    // No cooldown was started, so a genuine failure stays retryable.
    expect(resendButton()).toBeEnabled()
  })

  it('clears a stale failure line when the next attempt starts', async () => {
    mockApiRequest
      .mockRejectedValueOnce(
        Object.assign(new Error('boom'), { status: 500 })
      )
      .mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })
    expect(screen.getByText(/Sent · Check your inbox/)).toBeInTheDocument()
  })

  it('sends nothing on a second click while the control is parked', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())
    await waitFor(() => expect(resendButton()).toBeDisabled())
    await user.click(resendButton())

    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })

  it('uses the settings-row phrasing at compact density', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderControl('compact')

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByText('Sent · Again in 60s')).toBeInTheDocument()
    })
  })
})
