import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  SIGNUP_REQUEST_TIMEOUT_MS,
  useSignupRequestDeadline,
} from './signup-request-deadline'

describe('useSignupRequestDeadline', () => {
  const onExpire = vi.fn()

  beforeEach(() => {
    vi.useFakeTimers()
    onExpire.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function renderDeadline() {
    return renderHook(() => useSignupRequestDeadline(onExpire))
  }

  it('aborts the armed signal and then expires at 30 s, not before', () => {
    const { result } = renderDeadline()
    let abortedWhenExpired: boolean | undefined
    const signal = result.current.arm()
    onExpire.mockImplementation(() => {
      abortedWhenExpired = signal.aborted
    })

    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS - 1)
    expect(signal.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    expect(signal.aborted).toBe(true)
    expect(onExpire).toHaveBeenCalledTimes(1)
    expect(abortedWhenExpired).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does nothing once disarmed', () => {
    const { result } = renderDeadline()
    const signal = result.current.arm()

    result.current.disarm()
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

    expect(signal.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('replaces the previous deadline when armed again', () => {
    const { result } = renderDeadline()
    const first = result.current.arm()
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS / 2)

    const second = result.current.arm()
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS / 2)
    expect(first.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()

    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS / 2)
    expect(first.aborted).toBe(false)
    expect(second.aborted).toBe(true)
    expect(onExpire).toHaveBeenCalledTimes(1)
  })

  it('leaves no timer behind on unmount and does not abort the request', () => {
    const { result, unmount } = renderDeadline()
    const signal = result.current.arm()
    expect(vi.getTimerCount()).toBe(1)

    unmount()

    expect(vi.getTimerCount()).toBe(0)
    vi.advanceTimersByTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
    expect(signal.aborted).toBe(false)
    expect(onExpire).not.toHaveBeenCalled()
  })
})
