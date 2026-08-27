import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import {
  VERIFICATION_RESEND_COOLDOWN_SECONDS,
  formatResendStatus,
  isVerificationResendUnauthorized,
  resendStatusAnnouncement,
  useVerificationResendCooldown,
  verificationResendThrottle,
} from './useVerificationResendCooldown'

describe('verificationResendThrottle', () => {
  it('reads the wait off a throttled response and trusts it', () => {
    expect(verificationResendThrottle({ status: 429, retryAfter: 42 })).toEqual({
      seconds: 42,
      precision: 'exact',
    })
  })

  // The live path in production: CORS does not expose Retry-After, so every
  // throttled resend a real user hits lands here (PSY-1924).
  it('falls back to the standard cooldown, marked approximate, with no Retry-After', () => {
    expect(verificationResendThrottle({ status: 429 })).toEqual({
      seconds: VERIFICATION_RESEND_COOLDOWN_SECONDS,
      precision: 'approximate',
    })
  })

  it('treats anything that is not a throttle as a real failure', () => {
    expect(verificationResendThrottle({ status: 500 })).toBeNull()
    expect(verificationResendThrottle(new Error('boom'))).toBeNull()
    expect(verificationResendThrottle(null)).toBeNull()
    expect(verificationResendThrottle('429')).toBeNull()
  })
})

describe('isVerificationResendUnauthorized', () => {
  it('recognises a dead session', () => {
    expect(isVerificationResendUnauthorized({ status: 401 })).toBe(true)
    expect(isVerificationResendUnauthorized({ status: 403 })).toBe(true)
  })

  it('leaves throttles and real failures alone', () => {
    expect(isVerificationResendUnauthorized({ status: 429 })).toBe(false)
    expect(isVerificationResendUnauthorized({ status: 500 })).toBe(false)
    expect(isVerificationResendUnauthorized(null)).toBe(false)
  })
})

describe('formatResendStatus', () => {
  it('says nothing before anything has happened', () => {
    expect(
      formatResendStatus({ sent: false, secondsRemaining: 0, precision: 'exact' })
    ).toBeNull()
  })

  it('reports the confirmation and the wait independently', () => {
    expect(
      formatResendStatus({ sent: true, secondsRemaining: 60, precision: 'exact' })
    ).toBe('Sent · Check your inbox · Resend available in 60s')
    expect(
      formatResendStatus({ sent: true, secondsRemaining: 0, precision: 'exact' })
    ).toBe('Sent · Check your inbox')
    expect(
      formatResendStatus({ sent: false, secondsRemaining: 30, precision: 'exact' })
    ).toBe('Resend available in 30s')
  })

  // The honesty rule. An approximate wait is a local assumption, so quoting a
  // second count off it would be a precise-looking lie that ticks down
  // convincingly for a whole minute.
  it('quotes no second count for an approximate wait', () => {
    const status = formatResendStatus({
      sent: false,
      secondsRemaining: 60,
      precision: 'approximate',
    })

    expect(status).toBe('Resend available in about a minute')
    expect(status).not.toMatch(/\d/)
  })

  it('uses the settings-row phrasing at compact density', () => {
    expect(
      formatResendStatus({
        sent: true,
        secondsRemaining: 45,
        precision: 'exact',
        density: 'compact',
      })
    ).toBe('Sent · Again in 45s')
    expect(
      formatResendStatus({
        sent: false,
        secondsRemaining: 60,
        precision: 'approximate',
        density: 'compact',
      })
    ).toBe('Again in about a minute')
  })
})

describe('resendStatusAnnouncement', () => {
  it('says nothing before anything has happened', () => {
    expect(resendStatusAnnouncement(false, false)).toBeNull()
  })

  // The whole point of a separate announcement: a polite live region that
  // changed every second would announce sixty times over one cooldown.
  it('never carries a second count', () => {
    for (const [sent, cooling] of [
      [true, true],
      [true, false],
      [false, true],
    ] as const) {
      expect(resendStatusAnnouncement(sent, cooling)).not.toMatch(/\d/)
    }
  })

  // If this varied with the cooldown, the live region would change the moment
  // the wait ran out and read the confirmation back a minute after the send,
  // with no user action in between.
  it('does not change when the cooldown expires after a send', () => {
    expect(resendStatusAnnouncement(true, true)).toBe(
      resendStatusAnnouncement(true, false)
    )
  })

  it('distinguishes a confirmed send from a bare wait', () => {
    expect(resendStatusAnnouncement(true, false)).toContain(
      'Verification email sent'
    )
    expect(resendStatusAnnouncement(false, true)).toContain('not available yet')
  })
})

describe('useVerificationResendCooldown', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('is idle until a wait is started', () => {
    const { result } = renderHook(() => useVerificationResendCooldown())

    expect(result.current.secondsRemaining).toBe(0)
    expect(result.current.isCoolingDown).toBe(false)
    expect(result.current.precision).toBe('exact')
  })

  // The button is parked for the same span either way; only the copy differs,
  // so an approximate wait must still report real seconds internally.
  it('carries the precision the wait was started with', () => {
    const { result } = renderHook(() => useVerificationResendCooldown())

    act(() => result.current.start(60, 'approximate'))
    expect(result.current.precision).toBe('approximate')
    expect(result.current.isCoolingDown).toBe(true)
    expect(result.current.secondsRemaining).toBe(60)

    act(() => result.current.start(30))
    expect(result.current.precision).toBe('exact')
  })

  it('counts down and releases the control when the wait runs out', () => {
    const { result } = renderHook(() => useVerificationResendCooldown())

    act(() => result.current.start(3))
    expect(result.current.secondsRemaining).toBe(3)
    expect(result.current.isCoolingDown).toBe(true)

    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.secondsRemaining).toBe(2)

    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current.secondsRemaining).toBe(0)
    expect(result.current.isCoolingDown).toBe(false)
  })

  // Driven off an absolute deadline, so a tab that stops firing timers resumes
  // with the real remaining time rather than a counter frozen mid-wait.
  it('catches up after a gap in timer delivery', () => {
    const { result } = renderHook(() => useVerificationResendCooldown())

    act(() => result.current.start(60))
    act(() => {
      vi.advanceTimersByTime(45_000)
    })

    expect(result.current.secondsRemaining).toBe(15)
  })

  it('ignores a non-positive wait', () => {
    const { result } = renderHook(() => useVerificationResendCooldown())

    act(() => result.current.start(0))
    expect(result.current.isCoolingDown).toBe(false)

    act(() => result.current.start(Number.NaN))
    expect(result.current.isCoolingDown).toBe(false)
  })

  // A timer that outlives unmount fires setState into a torn-down jsdom and
  // fails the entire vitest run (PSY-1664), so this is a suite-wide guard.
  it('clears its interval on unmount', () => {
    const { result, unmount } = renderHook(() =>
      useVerificationResendCooldown()
    )

    act(() => result.current.start(60))
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    unmount()

    expect(vi.getTimerCount()).toBe(0)
  })
})
