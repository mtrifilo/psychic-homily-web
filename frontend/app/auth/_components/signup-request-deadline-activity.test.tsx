import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Activity, type ReactNode } from 'react'
import { renderHook } from '@testing-library/react'
import {
  SIGNUP_REQUEST_TIMEOUT_MS,
  useSignupRequestDeadline,
  type SignupRequestDeadline,
} from './signup-request-deadline'

// The deadline's owner is a page the router hides and shows again, which runs
// its effect cleanups on hide and reruns its effects on show.

describe('useSignupRequestDeadline across a hide and show', () => {
  const onExpire = vi.fn()

  beforeEach(() => {
    vi.useFakeTimers()
    onExpire.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function renderInActivity() {
    const activity: { mode: 'visible' | 'hidden' } = { mode: 'visible' }
    const view = renderHook(() => useSignupRequestDeadline(onExpire), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <Activity mode={activity.mode}>{children}</Activity>
      ),
    })
    return {
      ...view,
      hide() {
        activity.mode = 'hidden'
        view.rerender()
      },
      show() {
        activity.mode = 'visible'
        view.rerender()
      },
    }
  }

  it('cancels the timer while hidden without aborting the request', () => {
    const { result, hide } = renderInActivity()
    const signal = result.current.arm()

    hide()

    expect(vi.getTimerCount()).toBe(0)
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
    expect(signal.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()
  })

  it('gives a request still under the deadline a fresh one once shown', () => {
    const { result, hide, show } = renderInActivity()
    const signal = result.current.arm()
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS / 2)
    hide()
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS)

    show()
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS - 1)
    expect(signal.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    expect(signal.aborted).toBe(true)
    expect(onExpire).toHaveBeenCalledTimes(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([
    ['disarmed', (deadline: SignupRequestDeadline) => deadline.disarm()],
    [
      'expired',
      () => {
        vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS)
      },
    ],
  ])('arms nothing on show once the deadline was %s', (_label, settle) => {
    const { result, hide, show } = renderInActivity()
    result.current.arm()
    settle(result.current)
    onExpire.mockClear()

    hide()
    show()

    expect(vi.getTimerCount()).toBe(0)
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
    expect(onExpire).not.toHaveBeenCalled()
  })

  it('leaves no timer behind when unmounted after a show', () => {
    const { result, hide, show, unmount } = renderInActivity()
    const signal = result.current.arm()
    hide()
    show()
    expect(vi.getTimerCount()).toBe(1)

    unmount()

    expect(vi.getTimerCount()).toBe(0)
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
    expect(signal.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()
  })
})
