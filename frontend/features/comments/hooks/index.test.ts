import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { ApiError } from '@/lib/api'
import { formatCommentSubmissionError, useAutoDismissError } from './index'

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

// PSY-608: auto-dismiss banner state for optimistic-rollback mutations.
// The hook keeps the rollback's silent cache restore but surfaces a brief
// "action was reverted" message so the user knows what happened.
describe('useAutoDismissError (PSY-608)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('starts with no error', () => {
    const { result } = renderHook(() => useAutoDismissError())
    expect(result.current.error).toBeNull()
  })

  it('exposes the error after show() is called', () => {
    const { result } = renderHook(() => useAutoDismissError(3000))
    const err = new Error('boom')

    act(() => {
      result.current.show(err)
    })

    expect(result.current.error).toBe(err)
  })

  it('clears the error after the timeout elapses', () => {
    const { result } = renderHook(() => useAutoDismissError(3000))

    act(() => {
      result.current.show(new Error('boom'))
    })
    expect(result.current.error).not.toBeNull()

    act(() => {
      vi.advanceTimersByTime(3000)
    })

    expect(result.current.error).toBeNull()
  })

  it('resets the timer when show() is called again before timeout', () => {
    const { result } = renderHook(() => useAutoDismissError(3000))

    act(() => {
      result.current.show(new Error('first'))
    })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    // Re-trigger before timeout — second error visible, timer reset.
    const second = new Error('second')
    act(() => {
      result.current.show(second)
    })
    expect(result.current.error).toBe(second)

    // Original timeout would have fired by now, but the reset delays it.
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current.error).toBe(second)

    // Full second-timeout window completes.
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.error).toBeNull()
  })

  it('clears the pending timeout on unmount (no setState on unmounted component)', () => {
    const { result, unmount } = renderHook(() => useAutoDismissError(3000))

    act(() => {
      result.current.show(new Error('boom'))
    })
    unmount()

    // Advancing past the dismiss window must not throw or warn.
    act(() => {
      vi.advanceTimersByTime(5000)
    })
  })
})
