import { describe, it, expect } from 'vitest'
import {
  NIGHT_EARTH_SOURCE_ID,
  NIGHT_EARTH_TILES,
  NIGHT_EARTH_TILE_HOST,
} from './nightEarthRaster'
import { PH_BASEMAP_SOURCE_ID } from './phBasemap'

/**
 * The host constant is written out rather than parsed from the template (no
 * URL parsing inside an error handler), so nothing but this test stops the two
 * from drifting: repointing the raster at another GIBS endpoint would
 * otherwise leave `basemap_host` naming a host that no longer serves a tile —
 * a telemetry tag that reads as fact and is wrong, which is worse than absent.
 */
describe('night-earth raster constants', () => {
  it('states the host the tile template actually points at', () => {
    // The {z}/{y}/{x} placeholders are only in the path, so the template
    // parses as a URL as-is.
    expect(new URL(NIGHT_EARTH_TILES).hostname).toBe(NIGHT_EARTH_TILE_HOST)
  })

  it('keeps the source id distinct from the vector source', () => {
    // Two sources sharing an id would collapse basemapTelemetry's per-source
    // throttle back into one slot — the first failure would silence the other.
    expect(NIGHT_EARTH_SOURCE_ID).not.toBe(PH_BASEMAP_SOURCE_ID)
  })
})
