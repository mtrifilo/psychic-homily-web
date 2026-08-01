import { describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { useBrowserTimezone } from './useBrowserTimezone'

function TimezoneProbe() {
  return <span>{useBrowserTimezone() ?? 'none'}</span>
}

describe('useBrowserTimezone', () => {
  it('resolves to undefined in a server render', () => {
    // The guarantee the server-rendered show list depends on (PSY-1624): the
    // server must key its prefetch on a value the client's hydration render
    // will also produce, and the viewer's zone is not that value. Reading
    // `Intl` directly here would emit the SERVER's zone.
    expect(renderToString(<TimezoneProbe />)).toContain('none')
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
