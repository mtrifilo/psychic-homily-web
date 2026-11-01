import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen, waitFor } from '@testing-library/react'
import * as Sentry from '@sentry/nextjs'
import { renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { PasskeyLoginButton } from './passkey-login'

const mockPush = vi.fn()
const mockSetUser = vi.fn()
const mockStartAuthentication = vi.fn()
const fetchMock = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startAuthentication: (...args: unknown[]) => mockStartAuthentication(...args),
}))

// The components must resolve the backend through lib/api-base like every
// other data request, never from NEXT_PUBLIC_API_URL on their own: unset, that
// read skips the same-origin /api proxy.
vi.mock('@/lib/api-base', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  API_BASE_URL: 'https://api-base.test',
}))

describe('PasskeyLoginButton', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockSetUser.mockReset()
    mockStartAuthentication.mockReset()
    fetchMock.mockReset()

    mockStartAuthentication.mockResolvedValue({ id: 'assertion-id' })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('redirects to returnTo after successful passkey login', async () => {
    const user = userEvent.setup()

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        options: {
          publicKey: { challenge: 'begin-challenge', rpId: 'localhost' },
        },
        challenge_id: 'challenge-id',
      }),
    })

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        user: {
          id: 42,
          email: 'test@example.com',
          first_name: 'Test',
          last_name: 'User',
          email_verified: true,
          is_admin: false,
        },
      }),
    })

    renderWithProviders(<PasskeyLoginButton returnTo="/library" />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/library')
    })
    // The begin endpoint wraps the options as the credentials.get() argument;
    // the library takes only the inner options.
    expect(mockStartAuthentication).toHaveBeenCalledWith({
      optionsJSON: { challenge: 'begin-challenge', rpId: 'localhost' },
    })
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      'https://api-base.test/auth/passkey/login/begin',
      'https://api-base.test/auth/passkey/login/finish',
    ])
  })

  // The envelope the backend writes for a failure: a 503 whose body says so.
  it('reports a 503 from finish and leaves the button usable', async () => {
    const user = userEvent.setup()
    const onError = vi.fn()

    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        options: { publicKey: { challenge: 'begin-challenge' } },
        challenge_id: 'challenge-id',
      }),
    })
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 503,
      json: async () => ({
        success: false,
        message: 'Service temporarily unavailable',
        error_code: 'SERVICE_UNAVAILABLE',
      }),
    })

    renderWithProviders(<PasskeyLoginButton onError={onError} />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(onError).toHaveBeenCalledTimes(1)
    })
    const reported = onError.mock.lastCall?.[0]
    expect(reported).toBeInstanceOf(AuthError)
    expect(reported).toMatchObject({
      message: 'Service temporarily unavailable',
      code: AuthErrorCode.SERVICE_UNAVAILABLE,
      status: 503,
    })
    expect(mockSetUser).not.toHaveBeenCalled()
    expect(mockPush).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /sign in with passkey/i })).toBeEnabled()
    })
  })

  // A refusal answers 200 with success false; the code travels with it so the
  // page can answer the refusal by code.
  it('reports a refusal with the code the server answered', async () => {
    const user = userEvent.setup()
    const onError = vi.fn()

    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        options: { publicKey: { challenge: 'begin-challenge' } },
        challenge_id: 'challenge-id',
      }),
    })
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: false,
        message: 'Server copy for the recoverable refusal',
        error_code: 'ACCOUNT_RECOVERABLE',
        request_id: 'req-1',
      }),
    })

    renderWithProviders(<PasskeyLoginButton onError={onError} />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(onError).toHaveBeenCalledTimes(1)
    })
    expect(onError.mock.lastCall?.[0]).toMatchObject({
      message: 'Server copy for the recoverable refusal',
      code: AuthErrorCode.ACCOUNT_RECOVERABLE,
      requestId: 'req-1',
    })
    expect(mockSetUser).not.toHaveBeenCalled()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('reports nothing when the viewer cancels the passkey prompt', async () => {
    const user = userEvent.setup()
    const onError = vi.fn()

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        options: { publicKey: { challenge: 'begin-challenge' } },
        challenge_id: 'challenge-id',
      }),
    })
    const cancelled = new Error('The operation either timed out or was not allowed.')
    cancelled.name = 'NotAllowedError'
    mockStartAuthentication.mockRejectedValueOnce(cancelled)

    renderWithProviders(<PasskeyLoginButton onError={onError} />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /sign in with passkey/i })).toBeEnabled()
    })
    expect(onError).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(Sentry.captureException).not.toHaveBeenCalled()
    expect(Sentry.addBreadcrumb).toHaveBeenCalledTimes(1)
    expect(vi.mocked(Sentry.addBreadcrumb).mock.lastCall?.[0]).toMatchObject({
      category: 'passkey',
      level: 'info',
      data: { code: 'NotAllowedError' },
    })
  })

  describe('Sentry reporting', () => {
    const BEGIN_OK = {
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        options: { publicKey: { challenge: 'begin-challenge' } },
        challenge_id: 'challenge-id',
      }),
    }

    async function signInAndWaitForError(onError: (error: AuthError) => void) {
      const user = userEvent.setup()
      renderWithProviders(<PasskeyLoginButton onError={onError} />)
      await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))
      await waitFor(() => {
        expect(onError).toHaveBeenCalledTimes(1)
      })
    }

    // A closed account is an answer the page has copy for, so it must not
    // reach Sentry as an application error.
    it.each([
      [AuthErrorCode.ACCOUNT_RECOVERABLE],
      [AuthErrorCode.ACCOUNT_INACTIVE],
    ])('records a %s refusal as one breadcrumb and no exception', async code => {
      const onError = vi.fn()
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          success: false,
          message: 'Server copy',
          error_code: code,
          request_id: 'req-1',
        }),
      })

      await signInAndWaitForError(onError)

      expect(onError.mock.lastCall?.[0]).toMatchObject({ code })
      expect(Sentry.captureException).not.toHaveBeenCalled()
      expect(Sentry.addBreadcrumb).toHaveBeenCalledTimes(1)
      expect(Sentry.addBreadcrumb).toHaveBeenCalledWith({
        category: 'passkey',
        level: 'info',
        message: 'Expected passkey refusal',
        data: { code, error_type: 'login_failed' },
      })
    })

    it.each([
      ['begin', [] as unknown[]],
      ['finish', [BEGIN_OK]],
    ])('reports a %s 503 as an exception at error level', async (_step, before) => {
      const onError = vi.fn()
      for (const response of before) fetchMock.mockResolvedValueOnce(response)
      fetchMock.mockResolvedValueOnce({
        ok: false,
        status: 503,
        json: async () => ({
          success: false,
          message: 'Service temporarily unavailable',
          error_code: 'SERVICE_UNAVAILABLE',
        }),
      })

      await signInAndWaitForError(onError)

      expect(Sentry.addBreadcrumb).not.toHaveBeenCalled()
      expect(Sentry.captureException).toHaveBeenCalledTimes(1)
      expect(Sentry.captureException).toHaveBeenCalledWith(
        expect.objectContaining({ code: AuthErrorCode.SERVICE_UNAVAILABLE }),
        { level: 'error', tags: { service: 'passkey-auth', error_type: 'login_failed' } }
      )
    })

    it('treats a non-Error value named NotAllowedError as the cancel', async () => {
      const user = userEvent.setup()
      const onError = vi.fn()
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      mockStartAuthentication.mockRejectedValueOnce({ name: 'NotAllowedError' })

      renderWithProviders(<PasskeyLoginButton onError={onError} />)
      await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

      await waitFor(() => expect(Sentry.addBreadcrumb).toHaveBeenCalledTimes(1))
      expect(Sentry.captureException).not.toHaveBeenCalled()
      expect(onError).not.toHaveBeenCalled()
    })

    it('reports any other non-Error throw at error level and as UNKNOWN', async () => {
      const onError = vi.fn()
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      mockStartAuthentication.mockRejectedValueOnce('boom')

      await signInAndWaitForError(onError)

      expect(onError.mock.lastCall?.[0]).toMatchObject({
        message: 'An unexpected error occurred',
        code: AuthErrorCode.UNKNOWN,
      })
      expect(Sentry.addBreadcrumb).not.toHaveBeenCalled()
      expect(Sentry.captureException).toHaveBeenCalledWith('boom', {
        level: 'error',
        tags: { service: 'passkey-auth', error_type: 'login_failed' },
      })
    })

    it('reports a WebAuthn exception other than the refusal at error level', async () => {
      const onError = vi.fn()
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      const failure = new DOMException('The operation is insecure.', 'SecurityError')
      mockStartAuthentication.mockRejectedValueOnce(failure)

      await signInAndWaitForError(onError)

      expect(Sentry.addBreadcrumb).not.toHaveBeenCalled()
      expect(Sentry.captureException).toHaveBeenCalledTimes(1)
      expect(Sentry.captureException).toHaveBeenCalledWith(failure, {
        level: 'error',
        tags: { service: 'passkey-auth', error_type: 'login_failed' },
      })
    })
  })

  it('reports a begin failure with its code and status, without starting the ceremony', async () => {
    const user = userEvent.setup()
    const onError = vi.fn()

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 503,
      json: async () => ({
        success: false,
        message: 'Service temporarily unavailable',
        error_code: 'SERVICE_UNAVAILABLE',
      }),
    })

    renderWithProviders(<PasskeyLoginButton onError={onError} />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(onError).toHaveBeenCalledTimes(1)
    })
    expect(onError.mock.lastCall?.[0]).toMatchObject({
      message: 'Service temporarily unavailable',
      code: AuthErrorCode.SERVICE_UNAVAILABLE,
      status: 503,
    })
    expect(mockStartAuthentication).not.toHaveBeenCalled()
  })

  it('reports a begin response without options instead of starting the ceremony', async () => {
    const user = userEvent.setup()
    const onError = vi.fn()

    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: true, challenge_id: 'challenge-id' }),
    })

    renderWithProviders(<PasskeyLoginButton onError={onError} />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(onError).toHaveBeenCalledTimes(1)
    })
    expect(onError.mock.lastCall?.[0]).toMatchObject({
      message: 'Failed to start passkey login',
      code: AuthErrorCode.UNKNOWN,
    })
    expect(mockStartAuthentication).not.toHaveBeenCalled()
  })
})
