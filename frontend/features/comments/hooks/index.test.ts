import { describe, it, expect } from 'vitest'
import type { ApiError } from '@/lib/api'
import { formatCommentSubmissionError } from './index'

// PSY-589: the hook's 429 path must surface an inline error message
// (instead of silently swallowing the failure and clearing the form).
// formatCommentSubmissionError is the seam — the hook itself is a thin
// react-query mutation, but it exposes the api.ts error verbatim, and
// CommentThread/CommentCard pass that error through this formatter to
// drive the banner copy + countdown.
describe('formatCommentSubmissionError (PSY-589)', () => {
  it('returns null when there is no error (no banner)', () => {
    expect(formatCommentSubmissionError(null)).toBeNull()
    expect(formatCommentSubmissionError(undefined)).toBeNull()
  })

  it('uses Retry-After seconds for 429 countdown copy', () => {
    const err: ApiError = Object.assign(new Error('please wait 60 seconds...'), {
      status: 429,
      retryAfter: 60,
    })
    expect(formatCommentSubmissionError(err)).toBe(
      'Please wait 60s before commenting again.'
    )
  })

  it('falls back to capitalized server message when Retry-After is missing', () => {
    const err: ApiError = Object.assign(
      new Error('please wait 60 seconds between comments on the same entity'),
      { status: 429 }
    )
    expect(formatCommentSubmissionError(err)).toBe(
      'Please wait 60 seconds between comments on the same entity'
    )
  })

  it('falls back to a static message for 429 with neither header nor body', () => {
    const err: ApiError = Object.assign(new Error(''), { status: 429 })
    expect(formatCommentSubmissionError(err)).toBe(
      'Please wait a minute before commenting again.'
    )
  })

  it('capitalizes generic non-429 server messages (project copy convention)', () => {
    const err: ApiError = Object.assign(new Error('something went wrong'), {
      status: 500,
    })
    expect(formatCommentSubmissionError(err)).toBe('Something went wrong')
  })

  it('handles plain Error instances without ApiError fields', () => {
    expect(formatCommentSubmissionError(new Error('boom'))).toBe('Boom')
  })
})
