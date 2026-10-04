import { describe, it, expect, vi, afterEach } from 'vitest'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { ATLAS_COMPACT_VIEWPORT_QUERY } from '../atlasViewport'

const prefetchGlobeLand = vi.fn()
vi.mock('../basemap/globeSurface', () => ({
  prefetchGlobeLand: () => prefetchGlobeLand(),
}))
// The real module pulls in MapLibre; only the fact that it is requested matters.
vi.mock('./GlobeCanvas', () => ({ default: () => null }))

import { preloadAtlasMap } from './atlasMapPreload'

describe('preloadAtlasMap', () => {
  let restore: () => void = () => {}
  afterEach(() => restore())

  it('prefetches the land data on a compact viewport', () => {
    restore = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true }).restore
    preloadAtlasMap()
    expect(prefetchGlobeLand).toHaveBeenCalledTimes(1)
  })

  it('leaves the land data alone on a wide viewport, where the raster draws', () => {
    restore = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: false }).restore
    preloadAtlasMap()
    expect(prefetchGlobeLand).not.toHaveBeenCalled()
  })
})
