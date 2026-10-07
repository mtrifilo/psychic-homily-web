import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { PlaceableScene, VenuePin, VenueStackMarker } from './globeTypes'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { CanvasSectionErrorBoundary } from '@/components/shared/CanvasSectionErrorBoundary'
import { AtlasMapUnrecoverableError } from '../atlasViewport'
import { ATLAS_CONTEXT_RESTORE_DEADLINE_MS } from './atlasMapHealth'
import { sceneTooltipLabel } from './globeScale'

/**
 * MapLibre stubbed down to the seams the sheet layout drives: controls by
 * corner, DOM markers, layer click handlers and the camera report. WebGL is out
 * of scope; the browser walk in the PR is what proves the canvas paints.
 */
interface StubControl {
  kind: 'attribution' | 'navigation'
}
interface StubMarker {
  element: HTMLElement
  options: Record<string, unknown>
  lngLat: [number, number] | null
  removed: boolean
}

// Hoisted with the mock factory below, which runs before this file's body.
const stub = vi.hoisted(() => {
  const state = {
    maps: [] as InstanceType<typeof StubMap>[],
    markers: [] as StubMarker[],
    // MapLibre 6 builds a map with no painter when WebGL2 is refused.
    painterless: false,
  }

  class StubMap {
    handlers = new Map<string, ((event: unknown) => void)[]>()
    controls: { control: StubControl; position: string }[] = []
    container: HTMLElement
    canvas = document.createElement('canvas')
    painter: object | undefined = state.painterless ? undefined : {}
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disableRotation: vi.fn() }
    remove = vi.fn()
    flyTo = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    setLayoutProperty = vi.fn()
    setPaintProperty = vi.fn()
    constructor(public options: { container: HTMLElement }) {
      this.container = options.container
      state.maps.push(this)
    }
    private add(key: string, handler: (event: unknown) => void) {
      this.handlers.set(key, [...(this.handlers.get(key) ?? []), handler])
    }
    on(event: string, layerOrHandler: unknown, maybeHandler?: unknown) {
      if (typeof layerOrHandler === 'string') {
        this.add(`${event}:${layerOrHandler}`, maybeHandler as (e: unknown) => void)
      } else {
        this.add(event, layerOrHandler as (e: unknown) => void)
      }
      return this
    }
    once(event: string, handler: (e: unknown) => void) {
      this.add(event, handler)
      return this
    }
    fire(key: string, event?: unknown) {
      for (const h of this.handlers.get(key) ?? []) h(event)
    }
    addControl(control: StubControl, position: string) {
      this.controls.push({ control, position })
    }
    removeControl(control: StubControl) {
      this.controls = this.controls.filter((c) => c.control !== control)
    }
    getSource() {
      return { setData: vi.fn() }
    }
    getLayer() {
      return {}
    }
    getZoom() {
      return 13
    }
    getCenter() {
      return { lng: -87.63, lat: 41.88 }
    }
    getBounds() {
      return {
        getWest: () => -87.7,
        getSouth: () => 41.8,
        getEast: () => -87.5,
        getNorth: () => 41.95,
        contains: () => true,
      }
    }
    getContainer() {
      return this.container
    }
    getCanvas() {
      return this.canvas
    }
  }

  class AttributionControl {
    kind = 'attribution'
  }
  class NavigationControl {
    kind = 'navigation'
  }
  class Marker implements StubMarker {
    element: HTMLElement
    lngLat: [number, number] | null = null
    removed = false
    constructor(public options: Record<string, unknown> & { element: HTMLElement }) {
      this.element = options.element
      state.markers.push(this)
    }
    setLngLat(lngLat: [number, number]) {
      this.lngLat = lngLat
      return this
    }
    addTo(map: StubMap) {
      map.container.appendChild(this.element)
      return this
    }
    remove() {
      this.removed = true
      this.element.remove()
    }
  }

  return { state, StubMap, AttributionControl, NavigationControl, Marker }
})
type StubMap = InstanceType<typeof stub.StubMap>

vi.mock('maplibre-gl', () => ({
  Map: stub.StubMap,
  Marker: stub.Marker,
  AttributionControl: stub.AttributionControl,
  NavigationControl: stub.NavigationControl,
  setWorkerUrl: vi.fn(),
}))
vi.mock('maplibre-gl/dist/maplibre-gl.css', () => ({}))

import GlobeCanvas from './GlobeCanvas'

const CHICAGO: PlaceableScene = {
  city: 'Chicago',
  state: 'IL',
  slug: 'chicago-il',
  venue_count: 2,
  upcoming_show_count: 10,
  total_show_count: 10,
  // Zero, so no pulse ring starts an animation loop under jsdom.
  shows_this_week: 0,
  shows_calendar_week: 0,
  latitude: 41.88,
  longitude: -87.63,
} as PlaceableScene

const PINS: VenuePin[] = [
  { id: 1, name: 'Empty Bottle', lng: -87.63, lat: 41.88, upcomingShowCount: 9, nextShowLabel: '' },
  { id: 2, name: 'Hideout', lng: -87.63, lat: 41.88, upcomingShowCount: 4, nextShowLabel: '' },
  { id: 3, name: 'Thalia Hall', lng: -87.657, lat: 41.857, upcomingShowCount: 6, nextShowLabel: '' },
]
const STACK: VenueStackMarker = {
  key: '-87.630000,41.880000',
  lng: -87.63,
  lat: 41.88,
  venueIds: [1, 2],
  label: '2 venues · city centre',
}

type CanvasProps = Partial<Parameters<typeof GlobeCanvas>[0]>

function renderCanvas(props: CanvasProps = {}) {
  const all = {
    width: 390,
    height: 723,
    scenes: [CHICAGO],
    pov: { lat: 41.88, lng: -87.63, altitude: 1.6 },
    onSelect: vi.fn(),
    venues: PINS,
    cityLabel: 'Chicago, IL',
    ...props,
  }
  const utils = render(<GlobeCanvas {...all} />)
  const map = stub.state.maps[stub.state.maps.length - 1]
  act(() => map.fire('style.load'))
  return { ...utils, map, props: all }
}

function attributionCorners(map: StubMap) {
  return map.controls
    .filter((c) => c.control.kind === 'attribution')
    .map((c) => c.position)
}

describe('GlobeCanvas sheet-layout seams', () => {
  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })

  it('docks the attribution bottom-left by default', () => {
    const { map } = renderCanvas()
    expect(attributionCorners(map)).toEqual(['bottom-left'])
    expect(screen.getByTestId('globe-cursor-wrap')).not.toHaveAttribute('data-atlas-credit')
  })

  it('moves the attribution top-left on request, keeping exactly one', () => {
    const { map, rerender, props } = renderCanvas({ attributionPosition: 'top-left' })
    expect(attributionCorners(map)).toEqual(['top-left'])
    expect(screen.getByTestId('globe-cursor-wrap')).toHaveAttribute('data-atlas-credit', 'top')

    rerender(<GlobeCanvas {...props} attributionPosition="bottom-left" />)
    expect(attributionCorners(map)).toEqual(['bottom-left'])
  })

  it('shows the back-to-globe control in city view only when asked', () => {
    const onBackToGlobe = vi.fn()
    const { rerender, props } = renderCanvas({ onBackToGlobe })
    fireEvent.click(screen.getByRole('button', { name: 'Back to globe' }))
    expect(onBackToGlobe).toHaveBeenCalledTimes(1)

    rerender(<GlobeCanvas {...props} cityLabel={null} />)
    expect(screen.queryByRole('button', { name: 'Back to globe' })).not.toBeInTheDocument()

    rerender(<GlobeCanvas {...props} onBackToGlobe={undefined} />)
    expect(screen.queryByRole('button', { name: 'Back to globe' })).not.toBeInTheDocument()
  })

  it('draws one counted marker per stack that reports its key', () => {
    const onVenueStackSelect = vi.fn()
    renderCanvas({ venueStacks: [STACK], onVenueStackSelect })
    const marker = screen.getByRole('button', { name: /2 venues · city centre/ })
    expect(marker).toHaveTextContent('2')
    expect(marker).toHaveAttribute('aria-label', '2 venues · city centre')
    fireEvent.click(marker)
    expect(onVenueStackSelect).toHaveBeenCalledWith(STACK.key)
  })

  it('routes a tap on a stacked pin to the stack, and a lone pin to the venue', () => {
    const onVenueSelect = vi.fn()
    const onVenueStackSelect = vi.fn()
    const { map } = renderCanvas({
      venueStacks: [STACK],
      onVenueStackSelect,
      onVenueSelect,
    })
    act(() => map.fire('click:venue-pins', { features: [{ properties: { id: 2 } }] }))
    expect(onVenueStackSelect).toHaveBeenCalledWith(STACK.key)
    expect(onVenueSelect).not.toHaveBeenCalled()

    act(() => map.fire('click:venue-pins', { features: [{ properties: { id: 3 } }] }))
    expect(onVenueSelect).toHaveBeenCalledWith(3)
  })

  it('opens a stacked pin as a venue when no stack handler is given', () => {
    const onVenueSelect = vi.fn()
    const { map } = renderCanvas({ onVenueSelect })
    act(() => map.fire('click:venue-pins', { features: [{ properties: { id: 2 } }] }))
    expect(onVenueSelect).toHaveBeenCalledWith(2)
  })

  it('names stacked pins by their marker, not by a member label', () => {
    renderCanvas({ venueStacks: [STACK], onVenueStackSelect: vi.fn() })
    const labels = stub.state.markers
      .filter((m) => !m.removed && m.element.tagName === 'DIV')
      .map((m) => m.element.textContent)
    expect(labels).toContain('Thalia Hall')
    expect(labels).not.toContain('Empty Bottle')
    expect(labels).not.toContain('Hideout')
  })

  it('reports the viewport bounds with each camera settle', () => {
    const onCameraSettle = vi.fn()
    const { map } = renderCanvas({ onCameraSettle })
    act(() => map.fire('moveend'))
    expect(onCameraSettle).toHaveBeenLastCalledWith({
      lng: -87.63,
      lat: 41.88,
      zoom: 13,
      bounds: { west: -87.7, south: 41.8, east: -87.5, north: 41.95 },
    })
  })
})

describe('GlobeCanvas pulse rings', () => {
  const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'
  let matchMedia: ReturnType<typeof installMatchMedia>

  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })
  afterEach(() => {
    matchMedia.restore()
    vi.restoreAllMocks()
  })

  it('stops the rings when reduced motion is turned on mid-session', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    const setData = vi.fn()
    vi.spyOn(stub.StubMap.prototype, 'getSource').mockReturnValue({ setData })
    vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(7)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    renderCanvas({ scenes: [{ ...CHICAGO, shows_this_week: 2 }] })

    const ringFeatureCounts = () =>
      setData.mock.calls.map(([fc]) => (fc as { features: unknown[] }).features.length)
    expect(ringFeatureCounts()).toContain(1)
    setData.mockClear()

    matchMedia.set(REDUCED_MOTION_QUERY, true)
    expect(cancel).toHaveBeenCalledWith(7)
    expect(ringFeatureCounts()).toContain(0)
    expect(ringFeatureCounts()).not.toContain(1)
  })

  it('stops the rings while the context is lost and restarts them on the restored style', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    const setData = vi.fn()
    vi.spyOn(stub.StubMap.prototype, 'getSource').mockReturnValue({ setData })
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(7)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const { map } = renderCanvas({ scenes: [{ ...CHICAGO, shows_this_week: 2 }] })
    expect(raf).toHaveBeenCalledTimes(1)

    act(() => map.fire('webglcontextlost'))
    expect(cancel).toHaveBeenCalledWith(7)
    act(() => map.fire('webglcontextrestored'))
    expect(raf).toHaveBeenCalledTimes(1)

    setData.mockClear()
    act(() => map.fire('style.load'))
    expect(raf).toHaveBeenCalledTimes(2)
    // The restored style gets the current scenes and rings again.
    const featureCounts = setData.mock.calls.map(
      ([fc]) => (fc as { features: unknown[] }).features.length,
    )
    expect(featureCounts.filter((n) => n === 1).length).toBeGreaterThanOrEqual(2)
  })

  it('starts the rings again when reduced motion is turned off', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    const setData = vi.fn()
    vi.spyOn(stub.StubMap.prototype, 'getSource').mockReturnValue({ setData })
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(7)
    renderCanvas({ scenes: [{ ...CHICAGO, shows_this_week: 2 }] })
    expect(raf).not.toHaveBeenCalled()

    matchMedia.set(REDUCED_MOTION_QUERY, false)
    expect(raf).toHaveBeenCalled()
    expect(
      setData.mock.calls.map(([fc]) => (fc as { features: unknown[] }).features.length),
    ).toContain(1)
  })
})

/**
 * The canvas inside the error boundary AtlasGlobe wraps it in, with a style
 * load fired or not. Console errors are silenced: React logs every error a
 * boundary catches.
 */
function renderInBoundary({
  styleLoaded,
  venues,
}: {
  styleLoaded: boolean
  venues?: VenuePin[]
}) {
  vi.spyOn(console, 'error').mockImplementation(() => {})
  const onError = vi.fn()
  const onCameraSettle = vi.fn()
  render(
    <CanvasSectionErrorBoundary sentryTag="atlas-map-test" onError={onError}>
      <GlobeCanvas
        width={390}
        height={723}
        scenes={[CHICAGO]}
        pov={{ lat: 41.88, lng: -87.63, altitude: 1.6 }}
        onSelect={vi.fn()}
        onCameraSettle={onCameraSettle}
        venues={venues}
      />
    </CanvasSectionErrorBoundary>,
  )
  const map = stub.state.maps[stub.state.maps.length - 1]
  if (styleLoaded) act(() => map.fire('style.load'))
  return { map, onError, onCameraSettle }
}

function expectFellBack(onError: ReturnType<typeof vi.fn>, failureClass: string) {
  expect(onError).toHaveBeenCalledTimes(1)
  expect(onError.mock.calls[0][0]).toBeInstanceOf(AtlasMapUnrecoverableError)
  expect(onError.mock.calls[0][0]).toMatchObject({ failureClass })
  expect(screen.queryByTestId('globe-cursor-wrap')).not.toBeInTheDocument()
}

describe('GlobeCanvas without a WebGL2 context', () => {
  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    stub.state.painterless = true
    sessionStorage.clear()
  })
  afterEach(() => {
    stub.state.painterless = false
    vi.restoreAllMocks()
  })

  it('throws to its error boundary when MapLibre comes up without a painter', () => {
    const { onError } = renderInBoundary({ styleLoaded: false })
    expectFellBack(onError, 'context-refused')
  })
})

describe('GlobeCanvas failures MapLibre reports outside React', () => {
  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('falls back when a lost context is not restored by the deadline', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1))
    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()

    act(() => vi.advanceTimersByTime(1))
    expectFellBack(onError, 'context-lost')
    expect(map.remove).toHaveBeenCalledTimes(1)
  })

  it('keeps the map when a lost context is restored', () => {
    const { map, onError, onCameraSettle } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS / 2))
    act(() => map.fire('webglcontextrestored'))
    act(() => map.fire('style.load'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10))

    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
    expect(map.remove).not.toHaveBeenCalled()
    onCameraSettle.mockClear()
    act(() => map.fire('moveend'))
    expect(onCameraSettle).toHaveBeenCalledTimes(1)
  })

  it('falls back when the restore gets no WebGL2 context', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    const refused = new Error('WebGL2 is required to display this map.')
    refused.name = 'GPUInitializationError'
    act(() => map.fire('error', { error: refused }))
    // MapLibre still fires the restored event after a refused restore.
    act(() => map.fire('webglcontextrestored'))
    expectFellBack(onError, 'context-restore-refused')
  })

  it('falls back when the context is lost before the style loads', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => map.fire('webglcontextlost'))
    expectFellBack(onError, 'context-lost-before-style')
  })

  it('falls back on an error event during the first render, before the style loads', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => map.fire('error', { error: new Error('style is not valid') }))
    expectFellBack(onError, 'style-load-failed')
  })

  it('keeps the map on an error event after the style loaded', () => {
    const { map, onError, onCameraSettle } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('error', { error: new Error('Cannot style non-existing layer') }))
    act(() => map.fire('error', { error: new Error('AJAXError: 503'), sourceId: 'nightEarth' }))

    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
    expect(map.remove).not.toHaveBeenCalled()
    onCameraSettle.mockClear()
    act(() => map.fire('moveend'))
    expect(onCameraSettle).toHaveBeenCalledTimes(1)
  })

  it('stops the health watch when the canvas unmounts', () => {
    const { map } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    // The watch's page listener: only its disposer removes one.
    const removeListener = vi.spyOn(document, 'removeEventListener')
    cleanup()
    expect(removeListener).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
  })

  /** Feature-state writes throw, as MapLibre's do while the style is down. */
  function styleDown(map: StubMap) {
    const throwing = () => {
      throw new TypeError("Cannot read properties of null (reading 'removeFeatureState')")
    }
    map.removeFeatureState.mockImplementation(throwing)
    map.setFeatureState.mockImplementation(throwing)
  }
  function styleUp(map: StubMap) {
    map.removeFeatureState.mockReset()
    map.setFeatureState.mockReset()
  }

  it('keeps the map through a restore with a scene dot hovered', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true })
    act(() =>
      map.fire('mousemove:scene-dots', {
        point: { x: 10, y: 10 },
        features: [{ properties: { slug: 'chicago-il', sortKey: 1 } }],
      }),
    )
    expect(map.setFeatureState).toHaveBeenCalledTimes(1)
    const tooltip = screen.getByText(sceneTooltipLabel(CHICAGO))
    expect(tooltip).toHaveStyle({ display: 'block' })

    styleDown(map)
    act(() => map.fire('webglcontextlost'))
    // The dot went with the style, and so does its tooltip.
    expect(tooltip).toHaveStyle({ display: 'none' })
    // During the loss the pointer leaves the dot; the restore's resize fires
    // movestart before the restored style loads.
    act(() => map.fire('mouseleave:scene-dots'))
    act(() => map.fire('movestart'))
    act(() => map.fire('webglcontextrestored'))
    styleUp(map)
    act(() => map.fire('style.load'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10))

    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
    expect(map.removeFeatureState).not.toHaveBeenCalled()
  })

  it('clears a hovered venue pin and keeps the map when the context is lost', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true, venues: PINS })
    act(() =>
      map.fire('mousemove:venue-pins', {
        point: { x: 10, y: 10 },
        features: [{ properties: { id: 3 } }],
      }),
    )
    expect(map.setFeatureState).toHaveBeenCalledTimes(1)
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    expect(tooltip).toHaveStyle({ display: 'block' })

    styleDown(map)
    act(() => map.fire('webglcontextlost'))

    // The pin went with the style, and so does its tooltip.
    expect(tooltip).toHaveStyle({ display: 'none' })
    expect(map.getCanvas().style.cursor).toBe('')
    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
  })
})
