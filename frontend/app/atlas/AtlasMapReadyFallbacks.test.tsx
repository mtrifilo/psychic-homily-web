import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/react'

// The signal is page-load state, so each test loads a fresh copy of it and of
// the page that arms it.
async function load() {
  vi.resetModules()
  const signal = await import('@/lib/atlasMapReady')
  const { default: AtlasPage } = await import('./page')
  return { signal, AtlasPage }
}

vi.mock('@/features/scenes/components/AtlasGlobe', () => ({
  AtlasGlobe: () => <div data-testid="atlas-globe" />,
}))

describe('the Atlas page arms the Atlas-ready fallbacks', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('releases the signal at the cap', async () => {
    const { signal, AtlasPage } = await load()
    render(<AtlasPage />)
    vi.advanceTimersByTime(signal.ATLAS_MAP_READY_CAP_MS - 1)
    expect(signal.isAtlasMapReady()).toBe(false)
    vi.advanceTimersByTime(1)
    expect(signal.isAtlasMapReady()).toBe(true)
  })

  it('releases the signal on the first pointer input', async () => {
    const { signal, AtlasPage } = await load()
    render(<AtlasPage />)
    window.dispatchEvent(new Event('pointerdown'))
    expect(signal.isAtlasMapReady()).toBe(true)
  })

  it('disarms when the page unmounts before either', async () => {
    const { signal, AtlasPage } = await load()
    const { unmount } = render(<AtlasPage />)
    unmount()
    window.dispatchEvent(new Event('pointerdown'))
    vi.advanceTimersByTime(signal.ATLAS_MAP_READY_CAP_MS)
    expect(signal.isAtlasMapReady()).toBe(false)
  })
})
