import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, waitFor } from '@testing-library/react'
import GlobeCanvas from './GlobeCanvas'
import type { PlaceableScene } from './globeTypes'
import { clearAtlasCamera } from './atlasCamera'
import { ATLAS_COMPACT_VIEWPORT_QUERY } from '../atlasViewport'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import * as globeSurface from '../basemap/globeSurface'
import { reportGlobePlacesFailure } from '../basemap/basemapTelemetry'

vi.mock('../basemap/basemapTelemetry', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../basemap/basemapTelemetry')>()),
  reportGlobePlacesFailure: vi.fn(),
}))

/**
 * GlobeCanvas's place labels with MapLibre stubbed to a flat projection and
 * markers that report a rendered box from it (text 6px per character, 12px
 * tall), so the collision pass runs against known geometry. The rules
 * themselves are globePlaces.test's; this pins the wiring: the data loads on
 * the light look, scene labels and dots win, and nothing draws on the full
 * look or outside the label zoom range.
 */
interface Handler {
  (e?: unknown): void
}

let maps: StubMapShape[] = []
let zoom = 2.4

interface StubMapShape {
  container: HTMLDivElement
  fire: (event: string) => void
}

const PANE = { width: 390, height: 731 }
// A flat projection of the near side. Eastern longitudes stand for the far
// side of the globe: each projects onto the near-side point 180 degrees west,
// and unprojecting that point lands there, as a globe's far side does.
const nearLng = (lng: number) => (lng > 0 ? lng - 180 : lng)
const project = ([lng, lat]: [number, number]) => ({
  x: 195 + (nearLng(lng) + 98) * 6,
  y: 365 - (lat - 39) * 6,
})

vi.mock('maplibre-gl', () => {
  class StubControl {
    constructor(public options: unknown) {}
  }
  class StubMarker {
    el: HTMLElement
    lngLat: [number, number] = [0, 0]
    constructor(
      public options: { element: HTMLElement; anchor?: string; offset?: [number, number] },
    ) {
      this.el = options.element
    }
    setLngLat(lngLat: [number, number]) {
      this.lngLat = lngLat
      return this
    }
    addTo(map: StubMapShape) {
      const { x, y } = project(this.lngLat)
      const width = (this.el.textContent ?? '').length * 6
      const height = 12
      const left = x - width / 2
      const top =
        this.options.anchor === 'top' ? y + (this.options.offset?.[1] ?? 0) : y - height / 2
      this.el.getBoundingClientRect = () =>
        ({ left, top, right: left + width, bottom: top + height, width, height }) as DOMRect
      map.container.appendChild(this.el)
      return this
    }
    remove() {
      this.el.remove()
    }
  }
  class StubMapImpl {
    handlers = new Map<string, Handler[]>()
    setLayoutProperty = vi.fn()
    setPaintProperty = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    addControl = vi.fn()
    removeControl = vi.fn()
    remove = vi.fn()
    flyTo = vi.fn()
    // A started map has a painter (MapLibre leaves it unset when WebGL2 is refused).
    painter: object | undefined = {}
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disableRotation: vi.fn() }
    canvas = document.createElement('canvas')
    container = document.createElement('div')

    constructor(public options: { style: { sources: Record<string, unknown>; layers: { id: string }[] } }) {
      Object.defineProperty(this.container, 'clientWidth', { value: PANE.width })
      Object.defineProperty(this.container, 'clientHeight', { value: PANE.height })
      document.body.appendChild(this.container)
      maps.push(this as unknown as StubMapShape)
    }
    on(event: string, layerOrHandler: unknown, maybeHandler?: unknown) {
      const handler = (typeof layerOrHandler === 'function' ? layerOrHandler : maybeHandler) as Handler
      const key = typeof layerOrHandler === 'string' ? `${event}:${layerOrHandler}` : event
      this.handlers.set(key, [...(this.handlers.get(key) ?? []), handler])
    }
    off(event: string, handler: Handler) {
      this.handlers.set(event, (this.handlers.get(event) ?? []).filter((h) => h !== handler))
    }
    once(event: string, handler: Handler) {
      this.on(event, handler)
    }
    fire(event: string) {
      for (const h of this.handlers.get(event) ?? []) h()
    }
    getLayer(id: string) {
      return this.options.style.layers.find((l) => l.id === id)
    }
    getSource(id: string) {
      return id in this.options.style.sources ? { setData: vi.fn() } : undefined
    }
    isStyleLoaded() {
      return true
    }
    areTilesLoaded() {
      return true
    }
    getZoom() {
      return zoom
    }
    getCenter() {
      return { lng: -98, lat: 39 }
    }
    getBounds() {
      return { contains: () => false, getWest: () => 0, getSouth: () => 0, getEast: () => 0, getNorth: () => 0 }
    }
    getCanvas() {
      return this.canvas
    }
    getContainer() {
      return this.container
    }
    // The stub's canvas is not in its container; its markers sit in the
    // container itself (StubMarker.addTo).
    getCanvasContainer() {
      return this.canvas
    }
    project(lngLat: [number, number]) {
      return project(lngLat)
    }
    unproject([x, y]: [number, number]) {
      return { lng: (x - 195) / 6 - 98, lat: (365 - y) / 6 + 39 }
    }
  }
  return {
    Map: StubMapImpl,
    AttributionControl: StubControl,
    NavigationControl: StubControl,
    Marker: StubMarker,
    setWorkerUrl: vi.fn(),
  }
})

const PHOENIX = { lng: -112.07, lat: 33.45 }

const SCENES = [
  {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    venue_count: 9,
    // Past the continental label threshold, so the scene is labelled.
    upcoming_show_count: 200,
    total_show_count: 300,
    shows_this_week: 0,
    shows_calendar_week: 0,
    latitude: PHOENIX.lat,
    longitude: PHOENIX.lng,
  },
] as unknown as PlaceableScene[]

// Rank 0 overlaps the Phoenix scene label but not its dot; rank 1 sits on
// the dot; rank 2 is clear; rank 3 is clear of the scene but its label would
// land on rank 2's.
const PLACES: GeoJSON.FeatureCollection = {
  type: 'FeatureCollection',
  features: [
    { name: 'Under', rank: 0, at: [-109.17, 30.5] },
    { name: 'On Phoenix', rank: 1, at: [PHOENIX.lng, PHOENIX.lat + 1] },
    { name: 'Clear City', rank: 2, at: [-90, 45] },
    { name: 'Crowded', rank: 3, at: [-90.5, 45] },
  ].map(({ name, rank, at }) => ({
    type: 'Feature',
    properties: { name, rank },
    geometry: { type: 'Point', coordinates: at },
  })),
}

const POV = { lat: 39, lng: -98, altitude: 1.8 }

function placeLabelTexts() {
  return [...document.querySelectorAll('[data-testid="atlas-place-label"]')].map(
    (el) => el.textContent,
  )
}

describe('GlobeCanvas place labels', () => {
  let restoreMatchMedia: () => void = () => {}
  beforeEach(() => {
    maps = []
    zoom = 2.4
    sessionStorage.clear()
    // Each unmount saves the camera for the next show; every case opens fresh.
    clearAtlasCamera()
    // Measured label size: 6px per character, 12px tall. A positioned marker
    // element carries its own rect (StubMarker.addTo), which shadows this.
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (
      this: Element,
    ) {
      const width = (this.textContent ?? '').length * 6
      return { left: 0, top: 0, right: width, bottom: 12, width, height: 12 } as DOMRect
    })
    // The place data through its loader (the loader caches its result for
    // the page load, so stubbing fetch would only reach the first case);
    // the boundary files fail, which leaves no line data to draw.
    vi.spyOn(globeSurface, 'loadGlobePlaces').mockResolvedValue({ data: PLACES })
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: false })))
  })
  afterEach(() => {
    restoreMatchMedia()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  async function showMap(compact: boolean, scenes: PlaceableScene[] = SCENES) {
    restoreMatchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: compact }).restore
    render(
      <GlobeCanvas
        width={PANE.width}
        height={PANE.height}
        scenes={scenes}
        pov={POV}
        onSelect={() => {}}
      />,
    )
    expect(maps).toHaveLength(1)
    await act(async () => {
      maps[0].fire('style.load')
      maps[0].fire('render')
    })
    return maps[0]
  }

  it('labels the clear place and drops the ones on a scene label, a scene dot or a kept label', async () => {
    await showMap(true)
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
    const label = document.querySelector<HTMLElement>('[data-testid="atlas-place-label"]')!
    expect(label.style.pointerEvents).toBe('none')
    expect(label.className).toContain('font-mono')
    // The scene label is still there: the scene won its spot.
    expect(document.querySelector('[data-testid="atlas-scene-label"]')?.textContent).toBe('Phoenix')
  })

  it('draws no place label on the full look and never asks for the place data', async () => {
    const load = vi.mocked(globeSurface.loadGlobePlaces)
    await showMap(false)
    await act(async () => {})
    expect(placeLabelTexts()).toEqual([])
    expect(load).not.toHaveBeenCalled()
  })

  it('clears the labels as soon as the zoom leaves the label range and lays them out again on the next settle', async () => {
    const map = await showMap(true)
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
    zoom = 6
    act(() => map.fire('zoom'))
    expect(placeLabelTexts()).toEqual([])
    zoom = 3
    act(() => map.fire('zoom'))
    act(() => map.fire('moveend'))
    expect(placeLabelTexts()).toEqual(['Clear City'])
  })

  it('shows labels up to, not including, the start of the street crossfade', async () => {
    const map = await showMap(true)
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
    zoom = 5.49
    act(() => map.fire('zoom'))
    act(() => map.fire('moveend'))
    expect(placeLabelTexts()).toEqual(['Clear City'])
    zoom = 5.5
    act(() => map.fire('zoom'))
    expect(placeLabelTexts()).toEqual([])
    act(() => map.fire('moveend'))
    expect(placeLabelTexts()).toEqual([])
  })

  it('lays the place labels out again when a zoom threshold brings in new scene labels at the settle', async () => {
    // 50 shows: a dot from the start, a label only once the camera is close
    // enough. The label lands on Clear City's; the dot does not.
    const mid = { ...SCENES[0], city: 'Mid', slug: 'mid-xx', upcoming_show_count: 50, longitude: -90, latitude: 47.83 }
    const map = await showMap(true, [SCENES[0], mid])
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
    expect(document.querySelectorAll('[data-testid="atlas-scene-label"]')).toHaveLength(1)
    // A reduced-motion jump: the zoom and the settle fire in one go, before
    // React has rebuilt the scene labels.
    zoom = 3.3
    act(() => {
      map.fire('zoom')
      map.fire('moveend')
    })
    await waitFor(() =>
      expect(document.querySelectorAll('[data-testid="atlas-scene-label"]')).toHaveLength(2),
    )
    expect(placeLabelTexts()).not.toContain('Clear City')
  })

  it('lets a scene label on the far side of the globe hold no spot', async () => {
    // Its label projects onto Clear City's, but from behind the globe.
    const farSide = { ...SCENES[0], city: 'Far Side', slug: 'far-side-xx', longitude: 90, latitude: 46.5 }
    await showMap(true, [SCENES[0], farSide])
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
  })

  const rect = (box: { left: number; top: number; right: number; bottom: number }) =>
    ({ ...box, width: box.right - box.left, height: box.bottom - box.top }) as DOMRect

  /**
   * The light look with a Drift button the page draws beside the canvas,
   * whose screen box is whatever `driftRect` returns at each read.
   */
  async function showMapBesideDrift(driftRect: () => DOMRect, scenes: PlaceableScene[] = SCENES) {
    const drift: { current: HTMLButtonElement | null } = { current: null }
    restoreMatchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true }).restore
    render(
      <div>
        <GlobeCanvas width={PANE.width} height={PANE.height} scenes={scenes} pov={POV} onSelect={() => {}} />
        <button
          type="button"
          ref={(button) => {
            drift.current = button
            if (button) button.getBoundingClientRect = driftRect
          }}
        >
          Drift
        </button>
      </div>,
    )
    await act(async () => {
      maps[0].fire('style.load')
      maps[0].fire('render')
    })
    return { map: maps[0], drift: drift.current! }
  }

  it('keeps place labels clear of the page chrome drawn beside the canvas', async () => {
    // First clear of every label, then moved over Clear City's (213,323 to
    // 273,335).
    let box = { left: 300, top: 600, right: 360, bottom: 630 }
    const { drift } = await showMapBesideDrift(() => rect(box))
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))

    box = { left: 200, top: 315, right: 290, bottom: 345 }
    act(() => drift.setAttribute('data-moved', 'true'))
    await waitFor(() => expect(placeLabelTexts()).toEqual([]))
  })

  it('reads the chrome once per layout change while a camera flight changes the scene labels', async () => {
    // Each scene's label comes in at its own zoom band on the way down
    // (labelMinCountForAltitude in globeScale.ts): 50 shows at z2.8, 20 at
    // z3.3, 5 at z4.0. Phoenix's 200 is labelled throughout.
    const scenes = [
      SCENES[0],
      { ...SCENES[0], city: 'Fifty', slug: 'fifty-xx', upcoming_show_count: 50, longitude: -80, latitude: 30 },
      { ...SCENES[0], city: 'Twenty', slug: 'twenty-xx', upcoming_show_count: 20, longitude: -120, latitude: 50 },
      { ...SCENES[0], city: 'Five', slug: 'five-xx', upcoming_show_count: 5, longitude: -75, latitude: 50 },
    ] as PlaceableScene[]
    const reads = vi.fn(() => rect({ left: 16, top: 669, right: 79.375, bottom: 707 }))
    const { map, drift } = await showMapBesideDrift(reads, scenes)
    await waitFor(() => expect(placeLabelTexts()).toContain('Clear City'))
    const sceneLabelCount = () => document.querySelectorAll('[data-testid="atlas-scene-label"]').length
    expect(sceneLabelCount()).toBe(1)
    expect(reads).toHaveBeenCalledTimes(1)

    const fly = async (zooms: number[]) => {
      for (const next of zooms) {
        zoom = next
        act(() => map.fire('zoom'))
        act(() => map.fire('moveend'))
        await act(async () => {})
      }
    }
    // Down through three label bands: the scene-label set changes three times.
    await fly([2.8, 3.0, 3.3, 3.5, 4.0, 4.5])
    expect(sceneLabelCount()).toBe(4)
    expect(reads).toHaveBeenCalledTimes(1)

    // One layout change in the chrome: one read.
    act(() => drift.setAttribute('data-moved', 'true'))
    await waitFor(() => expect(reads).toHaveBeenCalledTimes(2))

    // Back up through the same bands: three more changes to the set.
    await fly([3.5, 3.0, 2.4])
    expect(sceneLabelCount()).toBe(1)
    expect(reads).toHaveBeenCalledTimes(2)
  })

  it('reports a place file that could not be loaded and draws no place label', async () => {
    const load = vi
      .mocked(globeSurface.loadGlobePlaces)
      .mockResolvedValueOnce({ data: null, status: 503 })
    await showMap(true)
    await waitFor(() => expect(reportGlobePlacesFailure).toHaveBeenCalledWith(503))
    expect(load).toHaveBeenCalledTimes(1)
    expect(placeLabelTexts()).toEqual([])
  })
})

describe('GlobeCanvas place labels and the first full render', () => {
  it('asks for the place data only after the first full render', async () => {
    maps = []
    clearAtlasCamera()
    const load = vi.spyOn(globeSurface, 'loadGlobePlaces').mockResolvedValue({ data: PLACES })
    const { restore } = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
    try {
      render(
        <GlobeCanvas width={PANE.width} height={PANE.height} scenes={SCENES} pov={POV} onSelect={() => {}} />,
      )
      await act(async () => maps[0].fire('style.load'))
      expect(load).not.toHaveBeenCalled()
      await act(async () => maps[0].fire('render'))
      expect(load).toHaveBeenCalledTimes(1)
    } finally {
      restore()
      vi.restoreAllMocks()
      document.body.innerHTML = ''
    }
  })
})
