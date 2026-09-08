import { describe, it, expect } from 'vitest'
import { formatPasswordConfirmError } from './password-confirm-errors'

const copy = {
  fallback: 'Something went wrong',
  throttled: 'Too many attempts.',
}

describe('formatPasswordConfirmError', () => {
  it('keeps the server message for a non-429 failure', () => {
    // The shape the auth hooks throw for a rejected password: the backend
    // answers 200 with success:false, and the hook turns that into an AuthError
    // carrying status 400.
    const error = Object.assign(new Error('Password is incorrect'), {
      status: 400,
    })
    expect(formatPasswordConfirmError(error, copy)).toBe('Password is incorrect')
  })

  it('prints the wait in seconds when Retry-After is readable', () => {
    const error = Object.assign(new Error('Rate limit exceeded.'), {
      status: 429,
      retryAfter: 42,
    })
    expect(formatPasswordConfirmError(error, copy)).toBe(
      'Too many attempts. Try again in 42s.'
    )
  })

  it('names the window when retryAfter is absent, the deployed path', () => {
    // The deployed frontend calls the backend cross-origin, where Retry-After
    // is not exposed, so `retryAfter` is undefined for real users.
    const error = Object.assign(new Error('Rate limit exceeded.'), {
      status: 429,
    })
    expect(formatPasswordConfirmError(error, copy)).toBe(
      'Too many attempts. Try again in a minute.'
    )
  })

  it('names the window rather than printing a zero wait', () => {
    const error = Object.assign(new Error('Rate limit exceeded.'), {
      status: 429,
      retryAfter: 0,
    })
    expect(formatPasswordConfirmError(error, copy)).toBe(
      'Too many attempts. Try again in a minute.'
    )
  })

  it('falls back when the failure carries no message', () => {
    expect(formatPasswordConfirmError(new Error(''), copy)).toBe(
      'Something went wrong'
    )
  })

  it('returns the fallback for a missing error', () => {
    expect(formatPasswordConfirmError(null, copy)).toBe('Something went wrong')
  })
})
