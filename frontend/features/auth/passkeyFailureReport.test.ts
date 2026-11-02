import { describe, it, expect } from 'vitest'
import * as Sentry from '@sentry/nextjs'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { reportPasskeyFailure } from './passkeyFailureReport'

describe('reportPasskeyFailure', () => {
  it.each([
    [AuthErrorCode.ACCOUNT_RECOVERABLE],
    [AuthErrorCode.ACCOUNT_INACTIVE],
    [AuthErrorCode.REAUTH_REQUIRED],
  ])('records the %s refusal as an info breadcrumb, not an exception', code => {
    const refusal = new AuthError('Server copy', code, {
      requestId: 'req-1',
      status: 200,
    })

    reportPasskeyFailure(refusal, 'login_failed')

    expect(Sentry.captureException).not.toHaveBeenCalled()
    expect(Sentry.addBreadcrumb).toHaveBeenCalledTimes(1)
    // The code and the ceremony, and nothing that names the request or the
    // account.
    expect(Sentry.addBreadcrumb).toHaveBeenCalledWith({
      category: 'passkey',
      level: 'info',
      message: 'Expected passkey refusal',
      data: { code, error_type: 'login_failed' },
    })
  })

  it.each([
    ['a DOMException', () => new DOMException('not allowed', 'NotAllowedError')],
    ['a renamed Error', () => Object.assign(new Error('not allowed'), { name: 'NotAllowedError' })],
  ])('records the viewer refusing the prompt (%s) as an info breadcrumb', (_label, cancel) => {
    reportPasskeyFailure(cancel(), 'registration_failed')

    expect(Sentry.captureException).not.toHaveBeenCalled()
    expect(Sentry.addBreadcrumb).toHaveBeenCalledWith({
      category: 'passkey',
      level: 'info',
      message: 'Expected passkey refusal',
      data: { code: 'NotAllowedError', error_type: 'registration_failed' },
    })
  })

  it.each([
    ['an INVALID_CREDENTIALS refusal', () => new AuthError('Invalid passkey', AuthErrorCode.INVALID_CREDENTIALS)],
    ['a VALIDATION_FAILED refusal', () => new AuthError('Invalid credential response', AuthErrorCode.VALIDATION_FAILED)],
    ['a SERVICE_UNAVAILABLE refusal', () => new AuthError('Service temporarily unavailable', AuthErrorCode.SERVICE_UNAVAILABLE, { status: 503 })],
    ['an UNKNOWN refusal', () => new AuthError('Rate limit exceeded', AuthErrorCode.UNKNOWN, { status: 429 })],
    ['a plain Error', () => new Error('Failed to start passkey login')],
    ['a WebAuthn error other than the refusal', () => new DOMException('already registered', 'InvalidStateError')],
    ['a thrown non-Error', () => 'boom'],
  ])('reports %s as an exception at error level', (_label, failure) => {
    const error = failure()

    reportPasskeyFailure(error, 'registration_failed')

    expect(Sentry.addBreadcrumb).not.toHaveBeenCalled()
    expect(Sentry.captureException).toHaveBeenCalledTimes(1)
    expect(Sentry.captureException).toHaveBeenCalledWith(error, {
      level: 'error',
      tags: { service: 'passkey-auth', error_type: 'registration_failed' },
    })
  })
})
