import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { PasskeyRegisterButton } from './PasskeyRegisterButton'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { startRegistration } from '@simplewebauthn/browser'

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

// The components must resolve the backend through lib/api-base like every
// other data request, never from NEXT_PUBLIC_API_URL on their own: unset, that
// read skips the same-origin /api proxy.
vi.mock('@/lib/api-base', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  API_BASE_URL: 'https://api-base.test',
}))

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400,
    status,
    statusText: '',
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

const BEGIN_OK = {
  success: true,
  options: { publicKey: { challenge: 'register-challenge' } },
  challenge_id: 'challenge-1',
}

async function openAndSubmit() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /Add passkey/i }))
  await user.click(
    await screen.findByRole('button', { name: /^Add passkey$|Register/i })
  )
}

describe('PasskeyRegisterButton', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('hands the library the options inside the publicKey wrapper', async () => {
    const publicKey = { challenge: 'register-challenge', rp: { name: 'Psychic Homily', id: 'localhost' } }
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({ success: true, options: { publicKey }, challenge_id: 'challenge-1' })
      )
      .mockResolvedValueOnce(
        jsonResponse({ success: true, message: 'Passkey registered successfully' })
      ) as unknown as typeof fetch
    const onSuccess = vi.fn()

    renderWithProviders(<PasskeyRegisterButton onSuccess={onSuccess} />)
    await openAndSubmit()

    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
    expect(vi.mocked(startRegistration)).toHaveBeenCalledWith({ optionsJSON: publicKey })
    expect(vi.mocked(global.fetch).mock.calls.map((call) => call[0])).toEqual([
      'https://api-base.test/auth/passkey/register/begin',
      'https://api-base.test/auth/passkey/register/finish',
    ])
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
      .mockResolvedValueOnce(jsonResponse(BEGIN_OK))
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

  // The envelope the backend writes for a failure: a 503 whose body says so.
  it('reports a 503 from finish with its code and leaves the dialog usable', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(BEGIN_OK))
      .mockResolvedValueOnce(
        jsonResponse(
          {
            success: false,
            message: 'Service temporarily unavailable',
            error_code: 'SERVICE_UNAVAILABLE',
          },
          503
        )
      ) as unknown as typeof fetch
    const onError = vi.fn()
    const onSuccess = vi.fn()

    renderWithProviders(<PasskeyRegisterButton onError={onError} onSuccess={onSuccess} />)
    await openAndSubmit()

    await waitFor(() => expect(onError).toHaveBeenCalled())
    const reported = onError.mock.calls[0][0]
    expect(reported).toBeInstanceOf(AuthError)
    expect((reported as AuthError).code).toBe(AuthErrorCode.SERVICE_UNAVAILABLE)
    expect((reported as AuthError).message).toBe('Service temporarily unavailable')
    expect(onSuccess).not.toHaveBeenCalled()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Register passkey/i })).toBeEnabled()
    )
  })

  // apiRequest handles this for every other credential surface. Here it is this
  // component's job: a refusal from an edge rule or a protection page carries
  // no JSON, and parsing it blind would surface a SyntaxError with no remedy.
  it('reports a non-JSON refusal as an ordinary failure, not a parse error', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      headers: new Headers(),
      json: async () => {
        throw new SyntaxError("Unexpected token '<'")
      },
    } as unknown as Response) as unknown as typeof fetch
    const onError = vi.fn()

    renderWithProviders(<PasskeyRegisterButton onError={onError} />)
    await openAndSubmit()

    await waitFor(() => expect(onError).toHaveBeenCalled())
    const reported = onError.mock.calls[0][0]
    expect(reported).toBeInstanceOf(Error)
    expect((reported as Error).message).toBe(
      'Failed to start passkey registration'
    )
    expect((reported as Error).name).not.toBe('SyntaxError')
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

  // The generated response types declare message required, but a refusal that
  // never passed through the success envelope parses as JSON without one: an
  // empty object, or huma's problem+json error model (title, status, detail).
  // Each step then reports its own sentence.
  const UNENVELOPED_REFUSALS = [
    { shape: 'an empty JSON object', response: () => jsonResponse({}, 502) },
    {
      shape: 'the huma problem+json error model',
      response: () =>
        jsonResponse(
          { title: 'Unprocessable Entity', status: 422, detail: 'validation failed' },
          422
        ),
    },
  ]

  it.each(UNENVELOPED_REFUSALS)(
    'reports the begin step sentence for $shape',
    async ({ response }) => {
      global.fetch = vi.fn().mockResolvedValue(response()) as unknown as typeof fetch
      const onError = vi.fn()

      renderWithProviders(<PasskeyRegisterButton onError={onError} />)
      await openAndSubmit()

      await waitFor(() => expect(onError).toHaveBeenCalled())
      const reported = onError.mock.calls[0][0]
      expect(reported).toBeInstanceOf(Error)
      expect(reported).not.toBeInstanceOf(AuthError)
      expect((reported as Error).message).toBe('Failed to start passkey registration')
    }
  )

  it.each(UNENVELOPED_REFUSALS)(
    'reports the finish step sentence for $shape',
    async ({ response }) => {
      global.fetch = vi
        .fn()
        .mockResolvedValueOnce(jsonResponse(BEGIN_OK))
        .mockResolvedValueOnce(response()) as unknown as typeof fetch
      const onError = vi.fn()
      const onSuccess = vi.fn()

      renderWithProviders(<PasskeyRegisterButton onError={onError} onSuccess={onSuccess} />)
      await openAndSubmit()

      await waitFor(() => expect(onError).toHaveBeenCalled())
      const reported = onError.mock.calls[0][0]
      expect(reported).toBeInstanceOf(Error)
      expect(reported).not.toBeInstanceOf(AuthError)
      expect((reported as Error).message).toBe('Failed to register passkey')
      expect(onSuccess).not.toHaveBeenCalled()
    }
  )
})
