'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
// Namespace import only: maplibre-gl v6 has no default export (see
// maplibreWorker.ts).
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
// Aims the worker pool at the vendored copy before any Map is constructed.
import '@/features/scenes/components/maplibreWorker'
import { handleBasemapError } from '@/features/scenes/basemap/basemapTelemetry'
import {
  PH_BASEMAP_MIN_ZOOM,
  phBasemapFragment,
} from '@/features/scenes/basemap/phBasemap'
import {
  venuePinFeatures,
  venuePinPaint,
} from '@/features/scenes/components/venuePinLayer'
import { MiniAtlasSkeleton } from './MiniAtlasSkeleton'
import { MiniAtlasUnavailable } from './MiniAtlasUnavailable'
import {
  MINI_ATLAS_FIT_PADDING_PX,
  MINI_ATLAS_MAX_FIT_ZOOM,
  miniAtlasBounds,
  type MiniAtlasPin,
} from '../venueMiniAtlas'

const SOURCE_ID = 'rooms'
const LAYER_ID = 'room-pins'

/**
 * How faded a room with nothing booked draws.
 *
 * Quiet rooms stay on the map — they are rooms, and the table lists them — but
 * a reader scanning the pane for somewhere to go tonight should not have to
 * tell them apart by radius alone.
 */
const QUIET_PIN_OPACITY = 0.45

/**
 * The basemap's vector layers switch on at PH_BASEMAP_MIN_ZOOM, so the camera
 * never goes above it: a zoom that drew no streets would be a blank rectangle
 * with pins floating on it.
 */
const MIN_ZOOM = PH_BASEMAP_MIN_ZOOM

/**
 * The pane's street basemap, with the background opaque at every zoom the
 * camera can reach.
 *
 * The fragment's background ramp exists for the Atlas globe's descent, where
 * the background must stay transparent over the starfield. There is no globe
 * here, so the ramp is collapsed below MIN_ZOOM and the street style is simply
 * on.
 */
function miniAtlasStyle(): maplibregl.StyleSpecification {
  const basemap = phBasemapFragment(MIN_ZOOM - 1, MIN_ZOOM)
  return {
    version: 8,
    glyphs: basemap.glyphs,
    sources: {
      ...basemap.sources,
      [SOURCE_ID]: {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
        // feature-state needs a top-level id; the rooms' own ids are it.
        promoteId: 'id',
      },
    },
    layers: [
      ...basemap.layers,
      {
        id: LAYER_ID,
        type: 'circle',
        source: SOURCE_ID,
        paint: {
          ...venuePinPaint(),
          'circle-opacity': [
            'case',
            ['get', 'isQuiet'],
            QUIET_PIN_OPACITY,
            1,
          ],
          'circle-stroke-opacity': [
            'case',
            ['get', 'isQuiet'],
            QUIET_PIN_OPACITY,
            1,
          ],
        },
      },
    ],
  }
}

export interface VenueMiniAtlasProps {
  /** The rows to draw, ALREADY positioned by `miniAtlasPins`. */
  pins: readonly MiniAtlasPin[]
  /** The room under the pointer, from either the table or this map. */
  hoveredVenueId: number | null
  /** Reports a pin the pointer entered or left. The page owns the id. */
  onHoverVenue: (venueId: number | null) => void
  /** Pin click. The page scrolls the room's row into view and focuses it. */
  onSelectVenue: (venueId: number) => void
}

/**
 * The `/venues` mini Atlas: this page of rooms, on the Atlas's street basemap.
 *
 * Built on MapLibre directly rather than by mounting the Atlas canvas: that
 * surface is a globe with scene dots, drift and a rail, and none of it belongs
 * beside a city table. What IS shared is everything that decides how a room
 * reads — the basemap, the pin radius scale, and the pin paint — so the same
 * mark means the same thing on both surfaces.
 *
 * The canvas is hidden from assistive tech and cannot be tabbed into, and
 * every room on it is a row in the table beside it, which is where a keyboard
 * reaches them: the map is a second view of that list, never the only way to
 * something. MapLibre's own zoom controls remain focusable and carry their own
 * labels, so the pane's keyboard surface is those two buttons and nothing
 * else.
 */
export function VenueMiniAtlas({
  pins,
  hoveredVenueId,
  onHoverVenue,
  onSelectVenue,
}: VenueMiniAtlasProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [map, setMap] = useState<maplibregl.Map | null>(null)
  // A style that never loads is the failure the skeleton would otherwise hide
  // forever: `load` does not fire, so without this the pane pulses for the rest
  // of the session over a map that is never coming.
  const [failed, setFailed] = useState(false)

  // The handlers the map binds are bound ONCE, with the map. Reading them
  // through refs keeps a new callback identity from tearing the map down.
  const onHoverRef = useRef(onHoverVenue)
  onHoverRef.current = onHoverVenue
  const onSelectRef = useRef(onSelectVenue)
  onSelectRef.current = onSelectVenue

  // Plain create/remove, and deliberately NO init guard that survives a hide:
  // a surviving guard ref is the one pattern that broke the previous globe
  // under Cache Components (PSY-1284). A fresh map per mount is correct.
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const instance = new maplibregl.Map({
      container,
      style: miniAtlasStyle(),
      center: [0, 0],
      zoom: MIN_ZOOM,
      minZoom: MIN_ZOOM,
      maxZoom: 17,
      // The pane sits inside a scrolling page: a plain wheel over it has to
      // scroll the page, not zoom the map out from under the reader.
      cooperativeGestures: true,
      attributionControl: false,
    })

    // Registered first, so the style's own TileJSON fetch — the earliest thing
    // that can fail — is already covered. A failure BEFORE the style loads is
    // fatal to the pane (no basemap, no pins); one after it is a missing tile
    // on a map that already works, which the reader can see for themselves.
    instance.on('error', (event) => {
      handleBasemapError(event)
      if (!instance.isStyleLoaded()) setFailed(true)
    })

    // Bottom-LEFT and never covered: the OpenStreetMap credit is an ODbL
    // licensing requirement, not chrome.
    instance.addControl(
      new maplibregl.AttributionControl({ compact: false }),
      'bottom-left',
    )
    instance.addControl(
      new maplibregl.NavigationControl({
        showCompass: false,
        visualizePitch: false,
      }),
      'top-left',
    )
    instance.touchZoomRotate.disableRotation()
    // The canvas is hidden from assistive tech, so it must not be reachable by
    // keyboard either: an aria-hidden element that can hold focus strands it.
    instance.keyboard.disable()
    const canvas = instance.getCanvas()
    canvas.setAttribute('aria-hidden', 'true')
    canvas.setAttribute('tabindex', '-1')

    // `mousemove` fires at pointer rate while the cursor sits on a pin, so
    // both the cursor write and the report are guarded: the style never
    // changes after the first frame, and a repeat of the same id would push an
    // identical update through the page on every frame.
    let reported: number | null = null
    const handleEnter = () => {
      instance.getCanvas().style.cursor = 'pointer'
    }
    const handleMove = (
      event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      const id = event.features?.[0]?.id
      if (typeof id !== 'number' || id === reported) return
      reported = id
      onHoverRef.current(id)
    }
    const handleLeave = () => {
      instance.getCanvas().style.cursor = ''
      reported = null
      onHoverRef.current(null)
    }
    const handleClick = (
      event: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      const id = event.features?.[0]?.id
      if (typeof id === 'number') onSelectRef.current(id)
    }

    instance.on('mouseenter', LAYER_ID, handleEnter)
    instance.on('mousemove', LAYER_ID, handleMove)
    instance.on('mouseleave', LAYER_ID, handleLeave)
    instance.on('click', LAYER_ID, handleClick)
    instance.on('load', () => {
      setFailed(false)
      setMap(instance)
    })

    return () => {
      setFailed(false)
      // The applied hover belongs to THIS map instance; a new one starts with
      // no feature-state, so a carried id would describe a map that never had
      // it set.
      appliedHoverRef.current = null
      setMap((prev) => (prev === instance ? null : prev))
      instance.remove()
    }
  }, [])

  const features = useMemo(() => venuePinFeatures(pins), [pins])

  // The identity of the SET on screen, so the camera refits when the page or
  // the filter changes the rooms and leaves the old ones nowhere near the
  // frame — and does not refit when a re-render hands over the same rooms.
  const pinSetKey = useMemo(
    () => pins.map((p) => p.id).join(','),
    [pins],
  )

  useEffect(() => {
    const source = map?.getSource(SOURCE_ID) as
      | maplibregl.GeoJSONSource
      | undefined
    source?.setData(features)
  }, [map, features])

  useEffect(() => {
    if (!map) return
    const bounds = miniAtlasBounds(pins)
    if (!bounds) return
    map.fitBounds(bounds, {
      padding: MINI_ATLAS_FIT_PADDING_PX,
      maxZoom: MINI_ATLAS_MAX_FIT_ZOOM,
      animate: false,
    })
    // Keyed on the set, not on the array: the pins array is rebuilt on every
    // render of the page, and refitting on identity would fight the reader for
    // the camera.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [map, pinSetKey])

  // ONE hover id, owned by the page. The map never sets its own hover state
  // from a pointer event: it reports the pin and waits to be told, so a row
  // hover and a pin hover cannot light different rooms.
  const appliedHoverRef = useRef<number | null>(null)
  useEffect(() => {
    if (!map) return
    const previous = appliedHoverRef.current
    if (previous !== null) {
      map.removeFeatureState({ source: SOURCE_ID, id: previous }, 'hover')
    }
    if (hoveredVenueId !== null) {
      map.setFeatureState({ source: SOURCE_ID, id: hoveredVenueId }, { hover: true })
    }
    appliedHoverRef.current = hoveredVenueId
  }, [map, hoveredVenueId])

  return (
    <>
      {/* Held over the canvas until the style has painted, so the pane is never
          a flash of empty box. `map` IS the ready signal: it is set in the
          `load` handler, so nothing second-guesses when the skeleton lifts. */}
      {!map && !failed && <MiniAtlasSkeleton />}
      {!map && failed && <MiniAtlasUnavailable />}
      {/* Inline position/inset, NOT Tailwind classes: maplibre-gl.css sets
          `.maplibregl-map { position: relative }` on this node at map init,
          which ties with the `absolute` utility class and, since that
          stylesheet is lazy-loaded after globals.css, wins on order —
          collapsing the container to 0 height, at which point the canvas falls
          back to its 300px default. Inline style always wins. */}
      <div
        ref={containerRef}
        data-testid="venue-mini-atlas-canvas"
        className="ph-mini-atlas"
        style={{ position: 'absolute', inset: 0 }}
      />
    </>
  )
}

export default VenueMiniAtlas
