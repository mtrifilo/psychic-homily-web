import { describe, it, expect, vi } from 'vitest'

const { canvasModule } = vi.hoisted(() => ({ canvasModule: { requested: 0 } }))
const prefetchGlobeLand = vi.fn()
vi.mock('../basemap/globeSurface', () => ({
  prefetchGlobeLand: () => prefetchGlobeLand(),
}))
// The real module pulls in MapLibre. The factory runs when the module is
// first requested, which is what the preload exists to do.
vi.mock('./GlobeCanvas', () => {
  canvasModule.requested++
  return { default: () => null }
})

import { preloadAtlasMap } from './atlasMapPreload'

describe('preloadAtlasMap', () => {
  it('requests the canvas module and prefetches the land data for the light globe', async () => {
    preloadAtlasMap({ lightGlobe: true })
    expect(prefetchGlobeLand).toHaveBeenCalledTimes(1)
    await vi.waitFor(() => expect(canvasModule.requested).toBe(1))
  })

  it('leaves the land data alone for the full globe, where the raster draws', () => {
    prefetchGlobeLand.mockClear()
    preloadAtlasMap({ lightGlobe: false })
    expect(prefetchGlobeLand).not.toHaveBeenCalled()
  })
})

describe('preloadAtlasMap when the canvas module fails to load', () => {
  it('swallows the failure, leaving the error state to the dynamic() render path', async () => {
    vi.resetModules()
    let attempted = false
    vi.doMock('./GlobeCanvas', () => {
      attempted = true
      throw new Error('chunk failed to load')
    })
    const unhandled = vi.fn()
    process.on('unhandledRejection', unhandled)
    try {
      const { preloadAtlasMap: preload } = await import('./atlasMapPreload')
      expect(() => preload({ lightGlobe: false })).not.toThrow()
      await vi.waitFor(() => expect(attempted).toBe(true))
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(unhandled).not.toHaveBeenCalled()
    } finally {
      process.off('unhandledRejection', unhandled)
      vi.doUnmock('./GlobeCanvas')
    }
  })
})
