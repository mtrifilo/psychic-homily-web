import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  pageviewWithinDailyCap,
  syncInternalFlagFromUrl,
} from './InternalTrafficAnalytics'

const KEY = 'ph-internal-traffic'
const COUNT_KEY = 'ph-pv-count'

describe('syncInternalFlagFromUrl', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('sets the flag on ?internal=1', () => {
    syncInternalFlagFromUrl('?internal=1')
    expect(window.localStorage.getItem(KEY)).toBe('1')
  })

  it('clears the flag on ?internal=0', () => {
    window.localStorage.setItem(KEY, '1')
    syncInternalFlagFromUrl('?internal=0')
    expect(window.localStorage.getItem(KEY)).toBeNull()
  })

  // The default path must be inert: an ordinary visitor's URL carries no
  // internal param, and must neither set nor clear anything.
  it('leaves an existing flag untouched when the param is absent', () => {
    window.localStorage.setItem(KEY, '1')
    syncInternalFlagFromUrl('?city=phoenix')
    expect(window.localStorage.getItem(KEY)).toBe('1')
  })

  it('does not set the flag for an unrelated URL', () => {
    syncInternalFlagFromUrl('')
    expect(window.localStorage.getItem(KEY)).toBeNull()
  })

  // Any other value is ignored rather than treated as truthy — `?internal=true`
  // must not silently suppress analytics.
  it('ignores values other than 1 and 0', () => {
    syncInternalFlagFromUrl('?internal=true')
    expect(window.localStorage.getItem(KEY)).toBeNull()
  })

  it('does not throw when localStorage is unavailable', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    expect(() => syncInternalFlagFromUrl('?internal=1')).not.toThrow()
  })
})

describe('pageviewWithinDailyCap', () => {
  const TODAY = '2026-08-18'

  beforeEach(() => {
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('allows the first pageview of the day and starts the counter', () => {
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(JSON.parse(window.localStorage.getItem(COUNT_KEY)!)).toEqual({
      d: TODAY,
      n: 1,
    })
  })

  it('increments across successive pageviews', () => {
    pageviewWithinDailyCap(TODAY)
    pageviewWithinDailyCap(TODAY)
    pageviewWithinDailyCap(TODAY)
    expect(JSON.parse(window.localStorage.getItem(COUNT_KEY)!).n).toBe(3)
  })

  it('drops pageviews once the daily budget is spent', () => {
    window.localStorage.setItem(COUNT_KEY, JSON.stringify({ d: TODAY, n: 50 }))
    expect(pageviewWithinDailyCap(TODAY)).toBe(false)
    // A refused pageview must not grow the counter.
    expect(JSON.parse(window.localStorage.getItem(COUNT_KEY)!).n).toBe(50)
  })

  it('resets the budget when the UTC date rolls over', () => {
    window.localStorage.setItem(
      COUNT_KEY,
      JSON.stringify({ d: '2026-08-17', n: 50 })
    )
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    expect(JSON.parse(window.localStorage.getItem(COUNT_KEY)!)).toEqual({
      d: TODAY,
      n: 1,
    })
  })

  // Corrupt storage must count as a fresh day, not break analytics.
  it('treats unparseable or misshapen stored values as a fresh counter', () => {
    window.localStorage.setItem(COUNT_KEY, 'not json')
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
    window.localStorage.setItem(COUNT_KEY, JSON.stringify({ d: TODAY, n: 'x' }))
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
  })

  // Same fail-open posture as the internal flag: no storage, keep counting.
  it('fails open when localStorage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    expect(pageviewWithinDailyCap(TODAY)).toBe(true)
  })
})
