import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as Sentry from '@sentry/nextjs'
import { renderWithProviders } from '@/test/utils'
import { apiRequest } from '@/lib/api'
import {
  VerificationResend,
  VerificationResendButton,
  VerificationResendFailed,
  VerificationResendSessionExpired,
  VerificationResendStatus,
  useVerificationResendState,
  type ResendStatusFormat,
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

function throttle(retryAfter?: number): Error {
  return Object.assign(new Error('Rate limit exceeded.'), {
    status: 429,
    retryAfter,
  })
}

/** Holds the next send open until the test settles it. */
function deferredSend(): () => Promise<void> {
  let resolveSend: (value: { success: boolean }) => void = () => undefined
  mockApiRequest.mockReturnValueOnce(
    new Promise(resolve => {
      resolveSend = resolve
    })
  )
  return () =>
    act(async () => {
      resolveSend({ success: true })
    })
}

const shoutedFormat: ResendStatusFormat = (sent, secondsRemaining) =>
  `${sent ? 'SENT' : 'IDLE'} / ${secondsRemaining}`

function renderControl({
  format,
  pendingLabel,
}: { format?: ResendStatusFormat; pendingLabel?: string } = {}) {
  return renderWithProviders(
    <VerificationResend service="test_surface">
      <VerificationResendButton pendingLabel={pendingLabel}>
        Send it again
      </VerificationResendButton>
      <VerificationResendStatus
        format={format}
      />
      <VerificationResendSessionExpired>
        Session gone, in this surface&rsquo;s words.
      </VerificationResendSessionExpired>
      <VerificationResendFailed />
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
    ).toThrow(/must be used inside <VerificationResend>/)

    consoleError.mockRestore()
  })

  it('renders an empty live region and nothing else before any attempt', () => {
    renderControl()

    // Mounted empty up front so the first announcement lands in a region that
    // is already on the page.
    expect(screen.getByRole('status')).toBeEmptyDOMElement()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(resendButton()).toBeEnabled()
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

  it('parks a throttle for the wait Retry-After asked for, as a wait and not an error', async () => {
    mockApiRequest.mockRejectedValueOnce(throttle(25))
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
    expect(Sentry.captureException).not.toHaveBeenCalled()
  })

  it('parks a throttle with no readable Retry-After for the standard cooldown', async () => {
    mockApiRequest.mockRejectedValueOnce(throttle())
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByText('Resend available in 60s')).toBeInTheDocument()
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(resendButton()).toBeDisabled()
  })

  it.each([401, 403])(
    'renders the surface\'s own session-expired alert on a %i, without paging Sentry',
    async status => {
      mockApiRequest.mockRejectedValueOnce(
        Object.assign(new Error('unauthorized'), { status })
      )
      const user = userEvent.setup()
      renderControl()

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          'Session gone, in this surface’s words.'
        )
      })
      expect(
        screen.queryByText(/We could not send that email just now/)
      ).not.toBeInTheDocument()
      // An expiring cookie is an ordinary event, not something to page on-call for.
      expect(Sentry.captureException).not.toHaveBeenCalled()
      // Nothing was sent and nothing was throttled, so no wait is running.
      expect(resendButton()).toBeEnabled()
    }
  )

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
        level: 'error',
        tags: { service: 'test_surface', error_type: 'verification_email' },
      })
    )
    // No cooldown was started, so a genuine failure stays retryable.
    expect(resendButton()).toBeEnabled()
  })

  // useSendVerificationEmail turns a 200 body with `success: false` into an
  // AuthError, which is a real failure rather than a throttle or an expiry.
  it('treats a refused send in a 200 body as a genuine failure', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: false,
      message: 'Email service is not configured',
      error_code: 'SERVICE_UNAVAILABLE',
    })
    const user = userEvent.setup()
    renderControl()

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'We could not send that email just now.'
      )
    })
    expect(screen.queryByText(/not configured/)).not.toBeInTheDocument()
    expect(Sentry.captureException).toHaveBeenCalledTimes(1)
  })

  describe('an already-verified refusal', () => {
    const alreadyVerified = {
      success: false,
      message: 'Email is already verified',
      error_code: 'ALREADY_VERIFIED',
    }

    // The reader verified elsewhere; nothing is broken, so nobody is paged.
    it('is not reported, and keeps the generic line by default', async () => {
      mockApiRequest.mockResolvedValueOnce(alreadyVerified)
      const user = userEvent.setup()
      renderControl()

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          'We could not send that email just now. Please try again in a moment.'
        )
      })
      expect(Sentry.captureException).not.toHaveBeenCalled()
    })

    it('takes the surface\'s own words when it supplies them', async () => {
      mockApiRequest.mockResolvedValueOnce(alreadyVerified)
      const user = userEvent.setup()
      renderWithProviders(
        <VerificationResend service="test_surface">
          <VerificationResendButton>Send it again</VerificationResendButton>
          <VerificationResendFailed alreadyVerified="Already done, in this surface's words." />
        </VerificationResend>
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          "Already done, in this surface's words."
        )
      })
      expect(screen.queryByText(/try again/)).not.toBeInTheDocument()
    })

    it('lets a genuine failure on the next attempt fall back to the generic line', async () => {
      mockApiRequest
        .mockResolvedValueOnce(alreadyVerified)
        .mockRejectedValueOnce(Object.assign(new Error('boom'), { status: 500 }))
      const user = userEvent.setup()
      renderWithProviders(
        <VerificationResend service="test_surface">
          <VerificationResendButton>Send it again</VerificationResendButton>
          <VerificationResendFailed alreadyVerified="Already done." />
        </VerificationResend>
      )

      await user.click(resendButton())
      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent('Already done.')
      })
      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          'We could not send that email just now.'
        )
      })
    })
  })

  it('clears a stale failure line when the next attempt starts', async () => {
    mockApiRequest
      .mockRejectedValueOnce(Object.assign(new Error('boom'), { status: 500 }))
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

  // The disabled attribute stops a pointer, not a caller: a surface that wires
  // `resend` to its own control still cannot double-send.
  it('ignores a direct call to resend while a send is in flight', async () => {
    const settleSend = deferredSend()
    // A surface-owned control that is never disabled, so only the guard
    // inside `resend` can stop the second send.
    function OwnControl() {
      const { resend } = useVerificationResendState()
      return (
        <button type="button" onClick={() => void resend()}>
          Own control
        </button>
      )
    }
    const user = userEvent.setup()
    renderWithProviders(
      <VerificationResend service="test_surface">
        <OwnControl />
        <VerificationResendButton>Send it again</VerificationResendButton>
      </VerificationResend>
    )
    const ownControl = screen.getByRole('button', { name: 'Own control' })

    await user.click(ownControl)
    await waitFor(() => expect(resendButton()).toBeDisabled())
    await user.click(ownControl)

    expect(mockApiRequest).toHaveBeenCalledTimes(1)
    await settleSend()
  })

  it('swaps in the surface\'s in-flight label only while a send is pending', async () => {
    const settleSend = deferredSend()
    const user = userEvent.setup()
    renderControl({ pendingLabel: 'Sending...' })

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Sending...' })).toBeDisabled()
    })

    await settleSend()
    await waitFor(() => expect(resendButton()).toBeDisabled())
  })

  it('announces in the wording the surface passes, when it passes one', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderWithProviders(
      <VerificationResend service="test_surface">
        <VerificationResendButton>Send it again</VerificationResendButton>
        <VerificationResendStatus
          announce={sent => (sent ? 'Surface says sent.' : null)}
        />
      </VerificationResend>
    )

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Surface says sent.')
    })
  })

  it('renders the visible line in the wording the surface passes', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderControl({ format: shoutedFormat })

    await user.click(resendButton())

    await waitFor(() => {
      expect(screen.getByText('SENT / 60')).toBeInTheDocument()
    })
    // The wording is the surface's; the announcement is not.
    expect(screen.getByRole('status')).toHaveTextContent(
      'Verification email sent. Check your inbox.'
    )
  })

  describe('cooldown timing', () => {
    beforeEach(() => {
      vi.useFakeTimers({ shouldAdvanceTime: true })
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('releases the control when the wait runs out, keeping the confirmation', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      renderControl()

      await user.click(resendButton())
      await waitFor(() => expect(resendButton()).toBeDisabled())

      act(() => {
        vi.advanceTimersByTime(60_000)
      })

      expect(resendButton()).toBeEnabled()
      expect(screen.getByText('Sent · Check your inbox')).toBeInTheDocument()
      // Byte-identical once the wait ends, so nothing is re-announced unprompted.
      expect(screen.getByRole('status')).toHaveTextContent(
        'Verification email sent. Check your inbox.'
      )
    })
  })
})
