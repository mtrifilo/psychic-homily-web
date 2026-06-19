import { describe, it, expect, afterEach, vi } from 'vitest'
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
  // Capture document.cookie writes via a temporary accessor (jsdom's real
  // setter would parse/store and we want the raw written string). Asserts on
  // the attribute CONTRACT (presence), not the serialization order, so a
  // harmless attribute reorder doesn't break the test.
  function captureWrites(): string[] {
    const writes: string[] = []
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      get: () => '',
      set: (value: string) => writes.push(value),
    })
    return writes
  }

  afterEach(() => {
    // Drop the temporary accessor so the prototype's cookie impl is restored.
    delete (document as { cookie?: unknown }).cookie
    vi.unstubAllGlobals()
  })

  it('writes name=value with path, a long max-age, and SameSite=Lax', () => {
    const writes = captureWrites()

    setNavModeCookie('side')

    expect(writes).toHaveLength(1)
    expect(writes[0]).toContain('nav_mode=side')
    expect(writes[0]).toContain('path=/')
    expect(writes[0]).toContain(`max-age=${NAV_MODE_MAX_AGE_SECONDS}`)
    expect(writes[0]).toContain('SameSite=Lax')
  })

  it('writes the chosen value (top vs side)', () => {
    const writes = captureWrites()
    setNavModeCookie('top')
    expect(writes[0]).toContain('nav_mode=top')
    expect(writes[0]).not.toContain('nav_mode=side')
  })

  it('omits Secure over http (so local plain-HTTP dev can store it)', () => {
    vi.stubGlobal('location', { protocol: 'http:' })
    const writes = captureWrites()
    setNavModeCookie('side')
    expect(writes[0]).not.toContain('Secure')
  })

  it('adds Secure over https', () => {
    vi.stubGlobal('location', { protocol: 'https:' })
    const writes = captureWrites()
    setNavModeCookie('side')
    expect(writes[0]).toContain('; Secure')
  })
})
