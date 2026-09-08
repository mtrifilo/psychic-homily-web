import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { PasskeyRegisterButton } from './PasskeyRegisterButton'
import { AuthError, AuthErrorCode } from '@/lib/errors'

// This button is the one gated surface that does not go through apiRequest: it
// drives the WebAuthn ceremony with raw fetch and translates the response body
// itself. Its card can only offer the sign-in-again remedy if that translation
// keeps the error code, so the translation is what these tests drive, against
// the body the backend's refusal actually writes.

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: () => true,
  startRegistration: vi.fn(async () => ({
    id: 'cred-1',
    rawId: 'cred-1',
    type: 'public-key',
    response: { attestationObject: 'a', clientDataJSON: 'b' },
  })),
}))

vi.mock('@sentry/nextjs', () => ({ captureException: vi.fn() }))

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    headers: new Headers(),
    json: async () => body,
  } as Response
}

// The exact envelope shared.ReauthRequiredError writes: success, message,
// error_code, request_id.
const REAUTH_REFUSAL = {
  success: false,
  message: 'For security, sign in again before making this change.',
  error_code: 'REAUTH_REQUIRED',
  request_id: 'req-1',
}

async function openAndSubmit() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /Add passkey/i }))
  await user.click(
    await screen.findByRole('button', { name: /^Add passkey$|Register/i })
  )
}

describe('PasskeyRegisterButton error translation', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('carries the refusal code out of the begin step', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(jsonResponse(REAUTH_REFUSAL)) as unknown as typeof fetch
    const onError = vi.fn()

    renderWithProviders(<PasskeyRegisterButton onError={onError} />)
    await openAndSubmit()

    await waitFor(() => expect(onError).toHaveBeenCalled())
    const reported = onError.mock.calls[0][0]
    expect(reported).toBeInstanceOf(AuthError)
    expect((reported as AuthError).code).toBe(AuthErrorCode.REAUTH_REQUIRED)
  })

  it('carries the refusal code out of the finish step', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({
          success: true,
          options: { publicKey: {} },
          challenge_id: 'challenge-1',
        })
      )
      .mockResolvedValueOnce(
        jsonResponse(REAUTH_REFUSAL)
      ) as unknown as typeof fetch
    const onError = vi.fn()

    renderWithProviders(<PasskeyRegisterButton onError={onError} />)
    await openAndSubmit()

    await waitFor(() => expect(onError).toHaveBeenCalled())
    const reported = onError.mock.calls[0][0]
    expect(reported).toBeInstanceOf(AuthError)
    expect((reported as AuthError).code).toBe(AuthErrorCode.REAUTH_REQUIRED)
  })

  // A failure the backend does not type stays an ordinary Error, so the card
  // renders its own copy rather than offering a remedy that does not apply.
  it('leaves an untyped failure as a plain Error', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      jsonResponse({ success: false, message: 'Something broke' })
    ) as unknown as typeof fetch
    const onError = vi.fn()

    renderWithProviders(<PasskeyRegisterButton onError={onError} />)
    await openAndSubmit()

    await waitFor(() => expect(onError).toHaveBeenCalled())
    const reported = onError.mock.calls[0][0]
    expect(reported).toBeInstanceOf(Error)
    expect(reported).not.toBeInstanceOf(AuthError)
    expect((reported as Error).message).toBe('Something broke')
  })
})
