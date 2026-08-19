import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import type { BeforeSendEvent } from '@vercel/analytics/react'
import InternalTrafficAnalytics, {
  syncInternalFlagFromUrl,
} from './InternalTrafficAnalytics'
import {
  DAILY_PAGEVIEW_CAP,
  PAGEVIEW_COUNT_KEY,
} from '@/lib/analytics/pageviewDailyCap'

const KEY = 'ph-internal-traffic'

let capturedBeforeSend: ((event: BeforeSendEvent) => BeforeSendEvent | null) | undefined

vi.mock('@vercel/analytics/react', () => ({
  Analytics: (props: {
    beforeSend?: (event: BeforeSendEvent) => BeforeSendEvent | null
  }) => {
    capturedBeforeSend = props.beforeSend
    return null
  },
}))

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

// The composed beforeSend chain is behavior the unit tests above cannot see:
// it is where the internal flag, the daily cap, the event-type gate, and the
// real UTC date expression meet.
describe('InternalTrafficAnalytics beforeSend', () => {
  const pageview = { type: 'pageview', url: '/shows' } as BeforeSendEvent
  const customEvent = { type: 'event', url: '/shows' } as BeforeSendEvent

  beforeEach(() => {
    window.localStorage.clear()
    capturedBeforeSend = undefined
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  function mountAndGetBeforeSend() {
    render(<InternalTrafficAnalytics />)
    expect(capturedBeforeSend).toBeDefined()
    return capturedBeforeSend!
  }

  it('passes pageviews through and keys the counter on the real UTC date', () => {
    const beforeSend = mountAndGetBeforeSend()
    expect(beforeSend(pageview)).toBe(pageview)
    const stored = JSON.parse(
      window.localStorage.getItem(PAGEVIEW_COUNT_KEY)!
    ) as { d: string; n: number }
    // Catches a regression in the date expression (e.g. a slice that produces
    // a month key and turns the daily cap into a monthly one).
    expect(stored.d).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(stored.n).toBe(1)
  })

  it('drops pageviews past the daily cap but never custom events', () => {
    const beforeSend = mountAndGetBeforeSend()
    for (let i = 0; i < DAILY_PAGEVIEW_CAP; i++) {
      expect(beforeSend(pageview)).toBe(pageview)
    }
    expect(beforeSend(pageview)).toBeNull()
    // A future conversion event must not spend or be blocked by the
    // pageview budget.
    expect(beforeSend(customEvent)).toBe(customEvent)
  })

  it('drops everything, including custom events, for internal browsers', () => {
    window.localStorage.setItem(KEY, '1')
    const beforeSend = mountAndGetBeforeSend()
    expect(beforeSend(pageview)).toBeNull()
    expect(beforeSend(customEvent)).toBeNull()
  })
})
