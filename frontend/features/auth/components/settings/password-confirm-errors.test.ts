import { describe, it, expect } from 'vitest'
import {
  formatPasswordConfirmError,
  PASSWORD_CONFIRM_THROTTLED_SENTENCE,
} from './password-confirm-errors'
import { formatChangePasswordError } from './change-password'
import { formatDeleteAccountError } from './delete-account-dialog'

const copy = { fallback: 'Something went wrong' }
const throttled = PASSWORD_CONFIRM_THROTTLED_SENTENCE

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
      `${throttled} Try again in 42s.`
    )
  })

  it('names the window when retryAfter is absent, the deployed path', () => {
    // The deployed frontend calls the backend cross-origin, where Retry-After
    // is not exposed, so `retryAfter` is undefined for real users.
    const error = Object.assign(new Error('Rate limit exceeded.'), {
      status: 429,
    })
    expect(formatPasswordConfirmError(error, copy)).toBe(
      `${throttled} Try again in a minute.`
    )
  })

  it('names the window rather than printing a zero wait', () => {
    const error = Object.assign(new Error('Rate limit exceeded.'), {
      status: 429,
      retryAfter: 0,
    })
    expect(formatPasswordConfirmError(error, copy)).toBe(
      `${throttled} Try again in a minute.`
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

// One budget meters both surfaces, so a person can be throttled on one by
// traffic to the other and a line naming the surface would name the wrong
// cause. The sentence is one exported constant, so the surfaces cannot drift;
// this pins that each of them renders that constant rather than a copy of its
// current wording.
describe('every surface renders the shared throttle sentence', () => {
  const rateLimited = Object.assign(new Error('Rate limit exceeded.'), {
    status: 429,
    retryAfter: 30,
  })

  it.each([
    ['the deletion dialog', formatDeleteAccountError],
    ['the settings password form', formatChangePasswordError],
  ])('%s opens its throttle line with it', (_name, format) => {
    expect(format(rateLimited)).toBe(
      `${PASSWORD_CONFIRM_THROTTLED_SENTENCE} Try again in 30s.`
    )
  })
})
