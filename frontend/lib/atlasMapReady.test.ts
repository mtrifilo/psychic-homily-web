import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// The signal is module state that lives for the page load, so every test
// loads a fresh copy.
async function load() {
  vi.resetModules()
  return import('./atlasMapReady')
}

describe('atlasMapReady', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('starts unreleased and releases once', async () => {
    const m = await load()
    const listener = vi.fn()
    m.subscribeAtlasMapReady(listener)
    expect(m.isAtlasMapReady()).toBe(false)

    m.markAtlasMapReady()
    m.markAtlasMapReady()

    expect(m.isAtlasMapReady()).toBe(true)
    expect(listener).toHaveBeenCalledTimes(1)
    await expect(m.whenAtlasMapReady()).resolves.toBeUndefined()
  })

  it('does not resolve the promise before release', async () => {
    const m = await load()
    const settled = vi.fn()
    void m.whenAtlasMapReady().then(settled)
    await vi.advanceTimersByTimeAsync(m.ATLAS_MAP_READY_CAP_MS * 2)
    expect(settled).not.toHaveBeenCalled()
  })

  it('stops notifying an unsubscribed listener', async () => {
    const m = await load()
    const listener = vi.fn()
    const unsubscribe = m.subscribeAtlasMapReady(listener)
    unsubscribe()
    m.markAtlasMapReady()
    expect(listener).not.toHaveBeenCalled()
  })

  it('releases on its own at the cap after arming, and not before', async () => {
    const m = await load()
    m.armAtlasMapReadyFallbacks()
    vi.advanceTimersByTime(m.ATLAS_MAP_READY_CAP_MS - 1)
    expect(m.isAtlasMapReady()).toBe(false)
    vi.advanceTimersByTime(1)
    expect(m.isAtlasMapReady()).toBe(true)
  })

  it.each(['pointerdown', 'keydown', 'wheel'] as const)(
    'releases on the first %s after arming',
    async type => {
      const m = await load()
      m.armAtlasMapReadyFallbacks()
      window.dispatchEvent(new Event(type))
      expect(m.isAtlasMapReady()).toBe(true)
    }
  )

  it('ignores input and the cap once disarmed', async () => {
    const m = await load()
    const disarm = m.armAtlasMapReadyFallbacks()
    disarm()
    window.dispatchEvent(new Event('pointerdown'))
    vi.advanceTimersByTime(m.ATLAS_MAP_READY_CAP_MS)
    expect(m.isAtlasMapReady()).toBe(false)
  })

  it('removes its input listeners and timer once the map releases it', async () => {
    const m = await load()
    const remove = vi.spyOn(window, 'removeEventListener')
    m.armAtlasMapReadyFallbacks()
    m.markAtlasMapReady()
    expect(remove.mock.calls.map(([type]) => type).sort()).toEqual(
      [...m.ATLAS_MAP_READY_INPUT_EVENTS].sort()
    )
    expect(vi.getTimerCount()).toBe(0)
    remove.mockRestore()
  })

  it('arms nothing when the signal has already released', async () => {
    const m = await load()
    m.markAtlasMapReady()
    const add = vi.spyOn(window, 'addEventListener')
    m.armAtlasMapReadyFallbacks()
    expect(add).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
    add.mockRestore()
  })
})
