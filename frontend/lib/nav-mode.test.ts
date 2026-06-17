import { describe, it, expect, afterEach } from 'vitest'
import {
  parseNavMode,
  setNavModeCookie,
  DEFAULT_NAV_MODE,
  NAV_MODE_MAX_AGE_SECONDS,
} from './nav-mode'

describe('parseNavMode', () => {
  it('returns "side" only for the explicit side opt-in', () => {
    expect(parseNavMode('side')).toBe('side')
  })

  it('returns "top" for the explicit top value', () => {
    expect(parseNavMode('top')).toBe('top')
  })

  // The coercion is the safety contract: any missing / malformed / stale value
  // resolves to the top default rather than throwing or rendering a broken
  // shell (mirrors the backend users.nav_mode CHECK).
  it.each([undefined, null, '', 'sidebar', 'TOP', 'Side', 'bogus'])(
    'coerces %p to the default',
    (value) => {
      expect(parseNavMode(value)).toBe(DEFAULT_NAV_MODE)
    }
  )
})

describe('setNavModeCookie', () => {
  afterEach(() => {
    // Drop the temporary accessor so the prototype's cookie impl is restored.
    delete (document as { cookie?: unknown }).cookie
  })

  it('writes the cookie with path, a long max-age, and SameSite=Lax', () => {
    const writes: string[] = []
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      get: () => '',
      set: (value: string) => writes.push(value),
    })

    setNavModeCookie('side')
    setNavModeCookie('top')

    expect(writes).toEqual([
      `nav_mode=side; path=/; max-age=${NAV_MODE_MAX_AGE_SECONDS}; SameSite=Lax`,
      `nav_mode=top; path=/; max-age=${NAV_MODE_MAX_AGE_SECONDS}; SameSite=Lax`,
    ])
  })
})
