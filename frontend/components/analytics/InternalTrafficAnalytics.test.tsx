import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { syncInternalFlagFromUrl } from './InternalTrafficAnalytics'

const KEY = 'ph-internal-traffic'

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
