import * as maplibregl from 'maplibre-gl'
import { GLOBE_PLACE_LABEL_COLOR } from '../basemap/globeSurface'
import {
  type Box,
  type GlobePlace,
  facingPoint,
  pickPlaceLabels,
  placeLabelBudget,
  placeLabelsShowAt,
} from '../basemap/globePlaces'

// The board's place label: 9.5px in the app's mono face.
const PLACE_LABEL_FONT_PX = 9.5
const PLACE_LABEL_STYLE = [
  'pointer-events: none',
  'user-select: none',
  'white-space: nowrap',
  `color: ${GLOBE_PLACE_LABEL_COLOR}`,
  `font-size: ${PLACE_LABEL_FONT_PX}px`,
  'line-height: normal',
].join(';')

function placeLabelElement(name: string): HTMLDivElement {
  const el = document.createElement('div')
  el.textContent = name
  el.className = 'font-mono'
  el.dataset.testid = 'atlas-place-label'
  el.setAttribute('aria-hidden', 'true')
  el.style.cssText = PLACE_LABEL_STYLE
  return el
}

// Each place's rendered label size, measured the first time it is needed.
// The face and size are fixed, so it holds for the page load; keyed by the
// place object, so a fresh place list measures afresh.
const labelSizes = new WeakMap<GlobePlace, { width: number; height: number }>()

export interface PlaceLabelOptions {
  /** Labels show below this zoom (and from PLACE_LABEL_MIN_ZOOM up). */
  maxZoom: number
  /**
   * The screen boxes place labels must stay clear of (scene labels and
   * dots), relative to the map container, read at each layout.
   */
  obstacles: () => Box[]
}

/**
 * Draws the light globe's place labels on a live map as DOM markers and keeps
 * them current: laid out on each camera settle, cleared as soon as the zoom
 * leaves the label range. A label's box is its place's screen point plus the
 * label's rendered size (measured once per place), so a layout builds
 * markers only for the labels it keeps (globePlaces.ts picks them). The
 * collision rules hold with the camera at rest: during a gesture the kept
 * labels ride their places until the settle lays them out again. Returns the
 * teardown.
 */
export function mountPlaceLabels(
  map: maplibregl.Map,
  places: readonly GlobePlace[],
  { maxZoom, obstacles }: PlaceLabelOptions,
): () => void {
  const container = map.getContainer()
  let markers: maplibregl.Marker[] = []

  const clear = () => {
    for (const m of markers) m.remove()
    markers = []
  }

  // Measures the not-yet-measured places in one layout pass: every element is
  // attached before any is read.
  const measure = (unmeasured: readonly GlobePlace[]) => {
    if (unmeasured.length === 0) return
    const probes = unmeasured.map((place) => {
      const el = placeLabelElement(place.name)
      el.style.position = 'absolute'
      el.style.visibility = 'hidden'
      container.appendChild(el)
      return { place, el }
    })
    for (const { place, el } of probes) {
      const r = el.getBoundingClientRect()
      labelSizes.set(place, { width: r.width, height: r.height })
    }
    for (const { el } of probes) el.remove()
  }

  const layout = () => {
    const zoom = map.getZoom()
    if (!placeLabelsShowAt(zoom, maxZoom)) {
      clear()
      return
    }
    const width = container.clientWidth
    const height = container.clientHeight
    const budget = placeLabelBudget(zoom)
    const blockers = obstacles()

    const facing: { place: GlobePlace; x: number; y: number }[] = []
    for (const place of places) {
      const point = facingPoint(map, place.lng, place.lat, width, height)
      if (point) facing.push({ place, ...point })
    }
    measure(facing.filter(({ place }) => !labelSizes.has(place)).map(({ place }) => place))

    const candidates = facing.map(({ place, x, y }) => {
      const size = labelSizes.get(place) ?? { width: 0, height: 0 }
      return {
        place,
        box: {
          left: x - size.width / 2,
          top: y - size.height / 2,
          right: x + size.width / 2,
          bottom: y + size.height / 2,
        },
      }
    })
    const kept = pickPlaceLabels(
      candidates,
      blockers,
      { left: 0, top: 0, right: width, bottom: height },
      budget,
    )

    clear()
    markers = kept.map(({ place }) =>
      new maplibregl.Marker({
        element: placeLabelElement(place.name),
        anchor: 'center',
        opacityWhenCovered: '0',
      })
        .setLngLat([place.lng, place.lat])
        .addTo(map),
    )
  }

  // Leaving the label range mid-gesture clears at once; entering it waits for
  // the settle, where layout runs anyway.
  const handleZoom = () => {
    if (markers.length > 0 && !placeLabelsShowAt(map.getZoom(), maxZoom)) clear()
  }

  layout()
  map.on('moveend', layout)
  map.on('zoom', handleZoom)
  return () => {
    map.off('moveend', layout)
    map.off('zoom', handleZoom)
    clear()
  }
}
