import { describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { CANONICAL_FIRST_SCREEN_TIMEZONE } from '@/lib/canonicalTimezone'
import { useBrowserTimezone } from './useBrowserTimezone'

function TimezoneProbe() {
  return <span>{useBrowserTimezone()}</span>
}

describe('useBrowserTimezone', () => {
  it('resolves to the canonical zone in a server render', () => {
    // The guarantee the server-rendered show list depends on (PSY-1624): the
    // server must key its prefetch on a value the client's hydration render
    // will also produce, and the viewer's zone is not that value. Reading
    // `Intl` directly here would emit whatever zone the render host is set to,
    // which in CI and in production are not the same and neither is the
    // viewer's.
    expect(renderToString(<TimezoneProbe />)).toContain(
      CANONICAL_FIRST_SCREEN_TIMEZONE
    )
  })

  it('does not fall back to UTC', () => {
    // Not a tautology restating the constant: UTC is the API's own default and
    // the value this would have used by omitting the parameter. It rolls over
    // at 4 or 5 PM Pacific, so a UTC first screen drops tonight's US shows at
    // the peak hour for reading listings. PSY-1678 removes the parameter
    // entirely; until then this is what stops a "simplification" back to it.
    expect(CANONICAL_FIRST_SCREEN_TIMEZONE).not.toBe('UTC')
    expect(renderToString(<TimezoneProbe />)).not.toContain('UTC')
  })

  it('resolves to the environment zone on the client', () => {
    const { result } = renderHook(() => useBrowserTimezone())

    expect(result.current).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
    expect(result.current).toBeTruthy()
  })

  it('returns a stable value across re-renders', () => {
    // `useSyncExternalStore` re-invokes the snapshot getter and compares with
    // Object.is; a getter that minted a new object each call would loop.
    const { result, rerender } = renderHook(() => useBrowserTimezone())
    const first = result.current
    rerender()

    expect(result.current).toBe(first)
  })
})
