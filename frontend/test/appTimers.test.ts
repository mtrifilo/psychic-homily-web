import { describe, it, expect, vi, afterEach } from 'vitest'
import { trackAppTimers, isAppOwnedStack } from './appTimers'

// Captured from a ShowForm render under fake timers: the frame below the tracker
// is the scheduling call site, and that is what decides ownership.
const LIBRARY_STACK = `Error
    at /repo/frontend/test/appTimers.ts:70:16
    at LiteThrottler.maybeExecute (file:///repo/frontend/node_modules/@tanstack/pacer-lite/dist/lite-throttler.js:56:23)
    at file:///repo/frontend/node_modules/@tanstack/form-core/dist/esm/FormApi.js:39:9
    at Object.FieldApi.handleChange (file:///repo/frontend/node_modules/@tanstack/form-core/dist/esm/FieldApi.js:403:12)
    at /repo/frontend/features/shows/components/ShowForm.tsx:612:20`

const APP_STACK = `Error
    at /repo/frontend/test/appTimers.ts:70:16
    at /repo/frontend/lib/hooks/common/useDismissTimer.ts:64:24
    at Object.onSuccess (/repo/frontend/features/shows/components/ShowForm.tsx:530:15)
    at file:///repo/frontend/node_modules/@tanstack/form-core/dist/esm/FormApi.js:520:38`

describe('isAppOwnedStack', () => {
  it('attributes a timer to its scheduling frame, not to deeper app frames', () => {
    // App code triggered this one, but a dependency scheduled it.
    expect(isAppOwnedStack(LIBRARY_STACK)).toBe(false)
  })

  it('tracks a timer scheduled from first-party code', () => {
    expect(isAppOwnedStack(APP_STACK)).toBe(true)
  })

  it('treats a stack it cannot attribute as app-owned', () => {
    expect(isAppOwnedStack(undefined)).toBe(true)
    expect(isAppOwnedStack('Error')).toBe(true)
  })
})

describe('trackAppTimers', () => {
  let tracker: ReturnType<typeof trackAppTimers> | null = null

  afterEach(() => {
    tracker?.restore()
    tracker = null
    vi.useRealTimers()
  })

  it('counts a pending timeout and drops it when it is cleared', () => {
    vi.useFakeTimers()
    tracker = trackAppTimers()

    const id = setTimeout(() => {}, 1000)
    expect(tracker.pending()).toBe(1)
    expect(tracker.describe()).toContain('appTimers.test.ts')

    clearTimeout(id)
    expect(tracker.pending()).toBe(0)
  })

  it('drops a timeout once it fires', () => {
    vi.useFakeTimers()
    tracker = trackAppTimers()

    const ran = vi.fn()
    setTimeout(ran, 1000)
    expect(tracker.pending()).toBe(1)

    vi.advanceTimersByTime(1000)
    expect(ran).toHaveBeenCalledTimes(1)
    expect(tracker.pending()).toBe(0)
  })

  it('keeps an interval pending until it is cleared', () => {
    vi.useFakeTimers()
    tracker = trackAppTimers()

    const tick = vi.fn()
    const id = setInterval(tick, 100)
    vi.advanceTimersByTime(250)
    expect(tick).toHaveBeenCalledTimes(2)
    expect(tracker.pending()).toBe(1)

    clearInterval(id)
    expect(tracker.pending()).toBe(0)
  })

  it('restores the timer functions it wrapped', () => {
    vi.useFakeTimers()
    const beforeTimeout = globalThis.setTimeout
    const beforeClear = globalThis.clearTimeout
    const beforeInterval = globalThis.setInterval
    const local = trackAppTimers()
    expect(globalThis.setTimeout).not.toBe(beforeTimeout)

    local.restore()
    expect(globalThis.setTimeout).toBe(beforeTimeout)
    expect(globalThis.clearTimeout).toBe(beforeClear)
    expect(globalThis.setInterval).toBe(beforeInterval)
  })
})
