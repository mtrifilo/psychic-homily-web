import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  matchesSoftKeyboardViewport,
  SOFT_KEYBOARD_VIEWPORT_QUERY,
} from './softKeyboardViewport'

const originalMatchMedia = window.matchMedia

afterEach(() => {
  window.matchMedia = originalMatchMedia
})

function mockMatchMedia(matchingQueries: string[]) {
  const seen: string[] = []
  window.matchMedia = vi.fn((query: string) => {
    seen.push(query)
    return { matches: matchingQueries.includes(query), media: query } as MediaQueryList
  })
  return seen
}

describe('matchesSoftKeyboardViewport', () => {
  it('is true on a narrow or coarse-pointer viewport', () => {
    const seen = mockMatchMedia([SOFT_KEYBOARD_VIEWPORT_QUERY])

    expect(matchesSoftKeyboardViewport()).toBe(true)
    expect(seen).toEqual([SOFT_KEYBOARD_VIEWPORT_QUERY])
  })

  it('is false on a wide fine-pointer viewport', () => {
    mockMatchMedia([])

    expect(matchesSoftKeyboardViewport()).toBe(false)
  })

  it('is false where matchMedia is unavailable', () => {
    // @ts-expect-error deliberately removing the API the guard exists for
    window.matchMedia = undefined

    expect(matchesSoftKeyboardViewport()).toBe(false)
  })
})
