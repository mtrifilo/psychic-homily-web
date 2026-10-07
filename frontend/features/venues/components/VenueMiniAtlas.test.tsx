import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import {
  resetStubMaps,
  stubMaps,
  type StubMap,
} from '@/test/miniAtlasMapStub'
import { ErrorProbe } from '@/test/ErrorProbe'
import { ATLAS_CONTEXT_RESTORE_DEADLINE_MS } from '@/features/scenes/components/atlasMapHealth'
import { VenueMiniAtlas } from './VenueMiniAtlas'
import { miniAtlasPins } from '../venueMiniAtlas'
import type { VenueWithShowCount } from '../types'

vi.mock('maplibre-gl', async () =>
  (await import('@/test/miniAtlasMapStub')).maplibreStubModule(),
)

function makeVenue(
  overrides: Partial<VenueWithShowCount> = {}
): VenueWithShowCount {
  return {
    id: 1,
    slug: 'a-room',
    name: 'A Room',
    city: 'Phoenix',
    state: 'AZ',
    address: '1 Main St',
    verified: true,
    upcoming_show_count: 4,
    latitude: 33.4,
    longitude: -112.0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

const ROOMS = [
  makeVenue({ id: 1, name: 'Busy Room', upcoming_show_count: 20 }),
  makeVenue({
    id: 2,
    name: 'Quiet Room',
    upcoming_show_count: 0,
    latitude: 33.6,
    longitude: -111.8,
  }),
]

function theMap(): StubMap {
  expect(stubMaps).toHaveLength(1)
  return stubMaps[0]
}

/** The style, then the first full render, as MapLibre fires them. */
function loadMap(map: StubMap) {
  act(() => {
    map.fire('style.load')
    map.fire('load')
  })
}

/** The circle layer the constructed style hands MapLibre. */
function pinLayer(map: StubMap) {
  const style = map.options.style as {
    layers: { id: string; paint: Record<string, unknown> }[]
  }
  const layer = style.layers.find(l => l.id === 'room-pins')
  expect(layer, 'the style carries a room-pin layer').toBeDefined()
  return layer!
}

function renderAtlas(
  props: Partial<React.ComponentProps<typeof VenueMiniAtlas>> = {}
) {
  const onHoverVenue = vi.fn()
  const onSelectVenue = vi.fn()
  const view = render(
    <VenueMiniAtlas
      pins={miniAtlasPins(ROOMS)}
      hoveredVenueId={null}
      onHoverVenue={onHoverVenue}
      onSelectVenue={onSelectVenue}
      {...props}
    />
  )
  return { ...view, onHoverVenue, onSelectVenue }
}

/** The map inside a boundary, with a page-owned hover id the test can move. */
function renderCaught(hoveredVenueId: number | null = null) {
  const caught: unknown[] = []
  const tree = (hovered: number | null) => (
    <ErrorProbe onCaught={error => caught.push(error)}>
      <VenueMiniAtlas
        pins={miniAtlasPins(ROOMS)}
        hoveredVenueId={hovered}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    </ErrorProbe>
  )
  const view = render(tree(hoveredVenueId))
  return {
    caught,
    setHovered: (hovered: number | null) => view.rerender(tree(hovered)),
  }
}

function lastFeatures(map: StubMap) {
  const calls = map.setData.mock.calls
  return (calls[calls.length - 1][0] as GeoJSON.FeatureCollection).features
}

describe('VenueMiniAtlas', () => {
  beforeEach(() => {
    resetStubMaps()
  })

  it('draws one pin per mappable row, sized by its upcoming count', () => {
    renderAtlas()
    const map = theMap()
    loadMap(map)

    const features = lastFeatures(map)
    expect(features).toHaveLength(2)
    expect(features[0].geometry).toEqual({
      type: 'Point',
      coordinates: [-112.0, 33.4],
    })
    const busy = features[0].properties?.radiusPx as number
    const quiet = features[1].properties?.radiusPx as number
    expect(busy).toBeGreaterThan(quiet)
  })

  it('marks a room with nothing booked as quiet', () => {
    renderAtlas()
    const map = theMap()
    loadMap(map)

    const features = lastFeatures(map)
    expect(features[0].properties?.isQuiet).toBe(false)
    expect(features[1].properties?.isQuiet).toBe(true)
  })

  it('draws a quiet room faded, mark and rim together', () => {
    renderAtlas()
    const paint = pinLayer(theMap()).paint

    // The paint, not just the flag: this is what actually mutes the pin.
    for (const key of ['circle-opacity', 'circle-stroke-opacity']) {
      const expression = paint[key] as unknown[]
      expect(expression[0], `${key} is a case expression`).toBe('case')
      expect(expression[1]).toEqual(['get', 'isQuiet'])
      // quiet value, then the value for everything else
      expect(expression[2]).toBeLessThan(1)
      expect(expression[3]).toBe(1)
    }
  })

  it('keeps the camera above the zoom where the basemap draws nothing', () => {
    renderAtlas()
    expect(theMap().options.minZoom).toBeGreaterThanOrEqual(5)
  })

  it('reports the room under a pin, and reports leaving it', () => {
    const { onHoverVenue } = renderAtlas()
    const map = theMap()
    loadMap(map)

    act(() => {
      map.fire('mousemove:room-pins', { features: [{ id: 2 }] })
    })
    expect(onHoverVenue).toHaveBeenCalledWith(2)

    act(() => {
      map.fire('mouseleave:room-pins')
    })
    expect(onHoverVenue).toHaveBeenLastCalledWith(null)
  })

  it('lights the pin the page says is hovered, and only that one', () => {
    const { rerender } = renderAtlas()
    const map = theMap()
    loadMap(map)

    rerender(
      <VenueMiniAtlas
        pins={miniAtlasPins(ROOMS)}
        hoveredVenueId={2}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    )
    expect(map.setFeatureState).toHaveBeenCalledWith(
      { source: 'rooms', id: 2 },
      { hover: true }
    )

    rerender(
      <VenueMiniAtlas
        pins={miniAtlasPins(ROOMS)}
        hoveredVenueId={1}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    )
    // The room that stopped being hovered is cleared in the same pass that
    // lights the new one, so two pins are never lit at once.
    expect(map.removeFeatureState).toHaveBeenCalledWith(
      { source: 'rooms', id: 2 },
      'hover'
    )
    expect(map.setFeatureState).toHaveBeenLastCalledWith(
      { source: 'rooms', id: 1 },
      { hover: true }
    )
  })

  it('hands a pin click back to the page as a room id', () => {
    const { onSelectVenue } = renderAtlas()
    const map = theMap()
    loadMap(map)

    act(() => {
      map.fire('click:room-pins', { features: [{ id: 2 }] })
    })
    expect(onSelectVenue).toHaveBeenCalledWith(2)
  })

  it('fits the rooms in view once the style has loaded', () => {
    renderAtlas()
    const map = theMap()
    expect(map.fitBounds).not.toHaveBeenCalled()

    loadMap(map)
    expect(map.fitBounds).toHaveBeenCalledWith(
      [
        [-112.0, 33.4],
        [-111.8, 33.6],
      ],
      expect.objectContaining({
        animate: false,
        padding: expect.any(Number),
        maxZoom: expect.any(Number),
      })
    )
    const [, options] = map.fitBounds.mock.calls[0] as [
      unknown,
      { padding: number; maxZoom: number },
    ]
    // Padding keeps an edge room's pin off the pane's border, and the cap stops
    // a city whose rooms share a block from landing at building zoom.
    expect(options.padding).toBeGreaterThan(0)
    expect(options.maxZoom).toBeLessThan(17)
  })

  it('refits when the page hands it a different set of rooms', () => {
    const { rerender } = renderAtlas()
    const map = theMap()
    loadMap(map)
    expect(map.fitBounds).toHaveBeenCalledTimes(1)

    const nextPage = miniAtlasPins([
      makeVenue({ id: 7, latitude: 40.0, longitude: -105.0 }),
    ])
    rerender(
      <VenueMiniAtlas
        pins={nextPage}
        hoveredVenueId={null}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    )

    expect(map.fitBounds).toHaveBeenCalledTimes(2)
    expect(map.fitBounds.mock.calls[1][0]).toEqual([
      [-105.0, 40.0],
      [-105.0, 40.0],
    ])
  })

  it('does not refit when the same rooms come back in a new array', () => {
    const { rerender } = renderAtlas()
    const map = theMap()
    loadMap(map)
    expect(map.fitBounds).toHaveBeenCalledTimes(1)

    // A re-render of the page rebuilds the pins array; the camera is the
    // reader's once it has been aimed.
    rerender(
      <VenueMiniAtlas
        pins={miniAtlasPins(ROOMS)}
        hoveredVenueId={null}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    )
    expect(map.fitBounds).toHaveBeenCalledTimes(1)
  })

  it('keeps a map that already works when a later tile fails', () => {
    const { queryByTestId } = renderAtlas()
    const map = theMap()
    loadMap(map)

    act(() => {
      map.fire('error', { error: new Error('one tile 500ed') })
    })

    expect(queryByTestId('venue-mini-atlas-canvas')).toBeInTheDocument()
  })

  it('keeps a plain wheel scrolling the page rather than zooming the map', () => {
    renderAtlas()
    expect(theMap().options.cooperativeGestures).toBe(true)
  })

  it('hides the canvas from assistive tech and keeps it out of the tab order', () => {
    renderAtlas()
    const map = theMap()
    loadMap(map)

    expect(map.canvas.getAttribute('aria-hidden')).toBe('true')
    expect(map.canvas.getAttribute('tabindex')).toBe('-1')
  })

  it('docks the required OpenStreetMap credit where nothing covers it', () => {
    renderAtlas()
    const positions = theMap().controls.map(c => c.position)
    expect(positions).toContain('bottom-left')
  })

  it('tears the map down when the pane goes away', () => {
    const { unmount } = renderAtlas()
    const map = theMap()
    loadMap(map)

    unmount()
    expect(map.remove).toHaveBeenCalled()
  })

  describe('a lost WebGL context', () => {
    afterEach(() => {
      vi.useRealTimers()
    })

    it('keeps the map when the browser restores the context', () => {
      vi.useFakeTimers()
      const { caught } = renderCaught()
      const map = theMap()
      loadMap(map)

      act(() => {
        map.fire('webglcontextlost')
      })
      act(() => {
        map.fire('webglcontextrestored')
        map.fire('style.load')
      })
      act(() => {
        vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 2)
      })

      expect(caught).toEqual([])
      expect(map.remove).not.toHaveBeenCalled()
    })

    it('writes no feature state while the style is gone, and lights the room once it is back', () => {
      const { caught, setHovered } = renderCaught()
      const map = theMap()
      loadMap(map)
      setHovered(2)
      expect(map.setFeatureState).toHaveBeenLastCalledWith(
        { source: 'rooms', id: 2 },
        { hover: true }
      )

      act(() => {
        map.fire('webglcontextlost')
      })
      map.setFeatureState.mockClear()
      map.removeFeatureState.mockClear()
      map.setData.mockClear()

      // A row hover while the context is lost: MapLibre has no style to
      // write to, and a write would throw.
      setHovered(1)
      expect(map.setFeatureState).not.toHaveBeenCalled()
      expect(map.removeFeatureState).not.toHaveBeenCalled()

      act(() => {
        map.fire('webglcontextrestored')
        map.fire('style.load')
      })

      // The restored style has no feature state, so nothing is cleared, and
      // the room the page hovers now is lit on it.
      expect(map.removeFeatureState).not.toHaveBeenCalled()
      expect(map.setFeatureState).toHaveBeenCalledTimes(1)
      expect(map.setFeatureState).toHaveBeenCalledWith(
        { source: 'rooms', id: 1 },
        { hover: true }
      )
      expect(map.setData).toHaveBeenCalledTimes(1)
      expect(caught).toEqual([])
    })

    it('does not refit the camera the reader may have moved when the context comes back', () => {
      renderCaught()
      const map = theMap()
      loadMap(map)
      expect(map.fitBounds).toHaveBeenCalledTimes(1)

      act(() => {
        map.fire('webglcontextlost')
      })
      act(() => {
        map.fire('webglcontextrestored')
        map.fire('style.load')
      })

      expect(map.fitBounds).toHaveBeenCalledTimes(1)
    })

    it('cancels the restore deadline when the pane goes away', () => {
      vi.useFakeTimers()
      const { unmount } = renderAtlas()
      const map = theMap()
      loadMap(map)
      act(() => {
        map.fire('webglcontextlost')
      })
      expect(vi.getTimerCount()).toBe(1)

      unmount()

      expect(vi.getTimerCount()).toBe(0)
    })
  })
})
