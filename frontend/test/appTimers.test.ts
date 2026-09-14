import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useDebouncedCallback } from 'use-debounce'
import { trackAppTimers, isAppOwnedStack } from './appTimers'

// Synthetic stacks in the shape this tracker reads: frame 1 is the tracker
// itself, frame 2 is the scheduling call site, and that frame alone decides
// ownership. Real stacks are exercised by the use-debounce case below.
const LIBRARY_STACK = `Error
    at /repo/frontend/test/appTimers.ts:1:1
    at LiteThrottler.maybeExecute (file:///repo/frontend/node_modules/@tanstack/pacer-lite/dist/lite-throttler.js:56:23)
    at file:///repo/frontend/node_modules/@tanstack/form-core/dist/esm/FormApi.js:39:9
    at /repo/frontend/features/shows/components/ShowForm.tsx:1:1`

const APP_STACK = `Error
    at /repo/frontend/test/appTimers.ts:1:1
    at /repo/frontend/lib/hooks/common/useDismissTimer.ts:1:1
    at file:///repo/frontend/node_modules/@tanstack/form-core/dist/esm/FormApi.js:520:38`

describe('isAppOwnedStack', () => {
  it('attributes a timer to its scheduling frame, not to deeper app frames', () => {
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
    expect(tracker.pendingCallSites()).toContain('appTimers.test.ts')

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

  // The fake clock installs more than setTimeout, and a leak through any of them
  // is the same defect.
  it('tracks an animation frame the same way', () => {
    vi.useFakeTimers()
    tracker = trackAppTimers()

    const id = requestAnimationFrame(() => {})
    expect(tracker.pending()).toBe(1)

    cancelAnimationFrame(id)
    expect(tracker.pending()).toBe(0)
  })

  // Attribution has to hold for the stacks this vitest config actually produces,
  // not only for the synthetic samples above.
  it('does not count a timer scheduled inside a dependency', () => {
    vi.useFakeTimers()
    tracker = trackAppTimers()

    const { result } = renderHook(() =>
      useDebouncedCallback((value: string) => value, 50)
    )
    result.current('call')

    expect(vi.getTimerCount()).toBeGreaterThan(0)
    expect(tracker.pending(), tracker.pendingCallSites()).toBe(0)
  })

  it('refuses a second install while one is active', () => {
    vi.useFakeTimers()
    tracker = trackAppTimers()

    expect(() => trackAppTimers()).toThrow(/already installed/)
  })

  it('restores the scheduler functions it wrapped', () => {
    vi.useFakeTimers()
    const beforeTimeout = globalThis.setTimeout
    const beforeClear = globalThis.clearTimeout
    const beforeInterval = globalThis.setInterval
    const beforeFrame = globalThis.requestAnimationFrame
    const local = trackAppTimers()
    expect(globalThis.setTimeout).not.toBe(beforeTimeout)

    local.restore()
    expect(globalThis.setTimeout).toBe(beforeTimeout)
    expect(globalThis.clearTimeout).toBe(beforeClear)
    expect(globalThis.setInterval).toBe(beforeInterval)
    expect(globalThis.requestAnimationFrame).toBe(beforeFrame)
  })
})
