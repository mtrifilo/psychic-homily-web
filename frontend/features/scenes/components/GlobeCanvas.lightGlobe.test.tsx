import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import GlobeCanvas from './GlobeCanvas'
import { ATLAS_COMPACT_VIEWPORT_QUERY } from '../atlasViewport'
import {
  GLOBE_LAND_DATA_URL,
  GLOBE_LAND_LAYER_ID,
  GLOBE_LAND_SOURCE_ID,
  GLOBE_OCEAN_LAYER_ID,
  NIGHT_EARTH_LAYER_ID,
} from '../basemap/globeSurface'

/**
 * GlobeCanvas's choice of globe surface, with MapLibre stubbed down to the
 * seams that choice drives: the style handed to the constructor, the layout
 * visibility set on a live map, and the land source's data. Whether the
 * hidden raster really costs no request is a browser fact, proven by the
 * network log of the perf harness, not here.
 */
interface StubMap {
  options: { style: StubStyle }
  fire: (event: string) => void
  setLayoutProperty: ReturnType<typeof vi.fn>
  landSetData: ReturnType<typeof vi.fn>
}

interface StubStyle {
  sources: Record<string, { data?: unknown }>
  layers: { id: string; layout?: { visibility?: string } }[]
}

let maps: StubMap[] = []

vi.mock('maplibre-gl', () => {
  class StubControl {
    constructor(public options: unknown) {}
  }
  class StubMapImpl {
    handlers = new Map<string, ((e?: unknown) => void)[]>()
    setLayoutProperty = vi.fn()
    landSetData = vi.fn()
    setPaintProperty = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    addControl = vi.fn()
    removeControl = vi.fn()
    remove = vi.fn()
    flyTo = vi.fn()
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disableRotation: vi.fn() }
    canvas = document.createElement('canvas')
    container = document.createElement('div')

    constructor(public options: { style: StubStyle }) {
      maps.push(this as unknown as StubMap)
    }
    on(event: string, layerOrHandler: unknown, maybeHandler?: unknown) {
      const handler = (typeof layerOrHandler === 'function'
        ? layerOrHandler
        : maybeHandler) as (e?: unknown) => void
      const key = typeof layerOrHandler === 'string' ? `${event}:${layerOrHandler}` : event
      this.handlers.set(key, [...(this.handlers.get(key) ?? []), handler])
    }
    once(event: string, handler: (e?: unknown) => void) {
      this.on(event, handler)
    }
    fire(event: string) {
      for (const h of this.handlers.get(event) ?? []) h()
    }
    getLayer(id: string) {
      return this.options.style.layers.find((l) => l.id === id)
    }
    getSource(id: string) {
      if (id === 'globeLand') return { setData: this.landSetData }
      return id in this.options.style.sources ? { setData: vi.fn() } : undefined
    }
    getZoom() {
      return 1.6
    }
    getCenter() {
      return { lng: 0, lat: 0 }
    }
    getBounds() {
      return { contains: () => false }
    }
    getCanvas() {
      return this.canvas
    }
    getContainer() {
      return this.container
    }
  }
  return {
    Map: StubMapImpl,
    AttributionControl: StubControl,
    NavigationControl: StubControl,
    Marker: StubControl,
    setWorkerUrl: vi.fn(),
  }
})

/** A `matchMedia` with a flippable answer for the compact query only. */
function installViewport(compact: boolean) {
  let matches = compact
  const listeners = new Set<() => void>()
  window.matchMedia = ((query: string) => ({
    get matches() {
      return query === ATLAS_COMPACT_VIEWPORT_QUERY ? matches : false
    },
    media: query,
    onchange: null,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia
  return {
    set(next: boolean) {
      matches = next
      act(() => listeners.forEach((fn) => fn()))
    },
  }
}

const POV = { lat: 39.5, lng: -98.35, altitude: 1.8 }

function renderCanvas() {
  return render(
    <GlobeCanvas width={390} height={731} scenes={[]} pov={POV} onSelect={() => {}} />,
  )
}

function theMap(): StubMap {
  expect(maps).toHaveLength(1)
  return maps[0]
}

function constructedVisibility(map: StubMap, layer: string) {
  return map.options.style.layers.find((l) => l.id === layer)?.layout?.visibility
}

function lastVisibilitySet(map: StubMap, layer: string) {
  return map.setLayoutProperty.mock.calls.filter((c) => c[0] === layer).at(-1)?.[2]
}

describe('GlobeCanvas globe surface', () => {
  const original = window.matchMedia
  beforeEach(() => {
    maps = []
    sessionStorage.clear()
  })
  afterEach(() => {
    window.matchMedia = original
  })

  it('builds a compact-viewport map without the night-earth raster', () => {
    installViewport(true)
    renderCanvas()
    const map = theMap()
    expect(constructedVisibility(map, NIGHT_EARTH_LAYER_ID)).toBe('none')
    expect(constructedVisibility(map, GLOBE_OCEAN_LAYER_ID)).toBe('visible')
    expect(constructedVisibility(map, GLOBE_LAND_LAYER_ID)).toBe('visible')
    expect(map.options.style.sources[GLOBE_LAND_SOURCE_ID].data).toBe(GLOBE_LAND_DATA_URL)
  })

  it('builds a wide-viewport map with the raster and no land request', () => {
    installViewport(false)
    renderCanvas()
    const map = theMap()
    expect(constructedVisibility(map, NIGHT_EARTH_LAYER_ID)).toBe('visible')
    expect(constructedVisibility(map, GLOBE_LAND_LAYER_ID)).toBe('none')
    expect(map.options.style.sources[GLOBE_LAND_SOURCE_ID].data).not.toBe(
      GLOBE_LAND_DATA_URL,
    )
  })

  it('switches the live map across the breakpoint without rebuilding it', () => {
    const viewport = installViewport(false)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))

    viewport.set(true)
    expect(lastVisibilitySet(map, NIGHT_EARTH_LAYER_ID)).toBe('none')
    expect(lastVisibilitySet(map, GLOBE_LAND_LAYER_ID)).toBe('visible')
    expect(map.landSetData).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL)

    viewport.set(false)
    expect(lastVisibilitySet(map, NIGHT_EARTH_LAYER_ID)).toBe('visible')
    expect(lastVisibilitySet(map, GLOBE_LAND_LAYER_ID)).toBe('none')
    expect(maps).toHaveLength(1)
  })

  it('does not refetch land on style load for a map built compact', () => {
    installViewport(true)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))
    expect(map.landSetData).not.toHaveBeenCalled()
    expect(lastVisibilitySet(map, NIGHT_EARTH_LAYER_ID)).toBe('none')
  })
})
