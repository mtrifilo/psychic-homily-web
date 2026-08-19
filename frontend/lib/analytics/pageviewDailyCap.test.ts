import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  DAILY_PAGEVIEW_CAP,
  PAGEVIEW_COUNT_KEY,
  pageviewWithinDailyCap,
} from './pageviewDailyCap'

const TODAY = '2026-08-18'

function storedCounter(): { d: string; n: number } {
  return JSON.parse(window.localStorage.getItem(PAGEVIEW_COUNT_KEY)!)
}

describe('pageviewWithinDailyCap', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('allows the first pageview of the day and starts the counter', () => {
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(storedCounter()).toEqual({ d: TODAY, n: 1 })
  })

  it('increments across successive pageviews', () => {
    pageviewWithinDailyCap(TODAY)
    pageviewWithinDailyCap(TODAY)
    pageviewWithinDailyCap(TODAY)
    expect(storedCounter().n).toBe(3)
  })

  it('drops pageviews once the daily budget is spent', () => {
    window.localStorage.setItem(
      PAGEVIEW_COUNT_KEY,
      JSON.stringify({ d: TODAY, n: DAILY_PAGEVIEW_CAP })
    )
    expect(pageviewWithinDailyCap(TODAY)).toBe(false)
    // A refused pageview must not grow the counter.
    expect(storedCounter().n).toBe(DAILY_PAGEVIEW_CAP)
  })

  it('resets the budget when the UTC date rolls over', () => {
    window.localStorage.setItem(
      PAGEVIEW_COUNT_KEY,
      JSON.stringify({ d: '2026-08-17', n: DAILY_PAGEVIEW_CAP })
    )
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(storedCounter()).toEqual({ d: TODAY, n: 1 })
  })

  // A poison value must be REPLACED, not preserved: if it survived, every
  // subsequent read would fail the same way and the browser would stay
  // uncapped for the life of the profile.
  it('overwrites an unparseable stored value and keeps counting', () => {
    window.localStorage.setItem(PAGEVIEW_COUNT_KEY, 'not json')
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(storedCounter()).toEqual({ d: TODAY, n: 1 })
  })

  it('overwrites a misshapen counter and keeps counting', () => {
    window.localStorage.setItem(
      PAGEVIEW_COUNT_KEY,
      JSON.stringify({ d: TODAY, n: 'x' })
    )
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(storedCounter()).toEqual({ d: TODAY, n: 1 })
  })

  // Out-of-range numbers must not wedge the cap: Infinity would otherwise
  // fail CLOSED for the whole day (the inverse of the fail-open posture) and
  // a negative count would widen the budget.
  it('treats out-of-range counts (Infinity, negative) as a fresh day', () => {
    window.localStorage.setItem(
      PAGEVIEW_COUNT_KEY,
      `{"d":"${TODAY}","n":1e999}`
    )
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(storedCounter()).toEqual({ d: TODAY, n: 1 })

    window.localStorage.setItem(
      PAGEVIEW_COUNT_KEY,
      JSON.stringify({ d: TODAY, n: -5 })
    )
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(storedCounter()).toEqual({ d: TODAY, n: 1 })
  })

  // Same fail-open posture as the internal flag: no storage, keep counting.
  it('fails open when localStorage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
  })
})
