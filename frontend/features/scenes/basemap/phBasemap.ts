// PH-branded dark basemap (PSY-1543) — the street-level vector style the
// Atlas globe crossfades into. The style itself lives in ph-dark-basemap.json
// (a complete standalone MapLibre style, versioned in-repo; see README.md in
// this directory for provenance, regeneration, attribution, and the
// Protomaps self-host fallback). This module adapts it into the fragment
// GlobeCanvas merges into its inline style.
import type {
  ExpressionSpecification,
  LayerSpecification,
  SourceSpecification,
  StyleSpecification,
} from 'maplibre-gl'
import phDarkBasemap from './ph-dark-basemap.json'

// resolveJsonModule types JSON literals as plain strings, not the style-spec
// literal unions — the cast is unavoidable. phDarkBasemap.test.ts validates
// the file against @maplibre/maplibre-gl-style-spec so the cast can't hide a
// malformed style.
const style = phDarkBasemap as unknown as StyleSpecification

// How far past fadeEnd the raster layer stays mounted. The fade reaches 0
// exactly AT fadeEnd; a layer `maxzoom` is exclusive, so cutting off at
// fadeEnd itself would drop the layer on the same frame the ramp lands and
// risk a one-frame seam on a slow tile. A hair past it unmounts the raster
// (no more GIBS fetches, no more compositing) only once it is provably
// invisible.
const RASTER_CUTOFF_MARGIN = 0.1

/**
 * The zoom at which the style's vector layers switch on — every non-background
 * layer in ph-dark-basemap.json is gated at or above this. Deliberately half a
 * zoom BELOW the crossfade start so the first street tiles are already in
 * flight (and drawn, under the still-opaque raster) when the fade begins.
 * Below it nothing vector renders and no vector tile is fetched.
 *
 * Exported so the callers that must agree with it — the atmosphere ramp in
 * GlobeCanvas and the minzoom guard in phDarkBasemap.test.ts — derive it
 * instead of repeating the literal.
 */
export const PH_BASEMAP_MIN_ZOOM = 5

/**
 * The style's one vector source id, and the host the style is CONFIGURED
 * against.
 *
 * Both are stated here rather than derived from the JSON at runtime: a
 * derivation would have to cope with a reshaped style by throwing (taking the
 * whole Atlas down for a telemetry label) or by silently returning a
 * placeholder. Instead phDarkBasemap.test.ts asserts the JSON still matches,
 * so a regenerated style that renames the source fails a test rather than
 * quietly aiming the failure signal at a source that no longer exists.
 *
 * STYLE host, not tile host, and the distinction matters. The source is a
 * TileJSON endpoint (`.../planet`); the tile URLs come from whatever that
 * endpoint returns at RUNTIME and are not in this repo, so no test can pin
 * them. basemapTelemetry uses this only as a best-effort fallback for the
 * `basemap_host` tag when the error carries no URL of its own — if OpenFreeMap
 * ever serves tiles from a different CDN host, that fallback would name the
 * configured host rather than the one that actually failed.
 *
 * Consumed by basemapTelemetry.ts, which reports a failure of THIS source and
 * ignores every other MapLibre error (PSY-1568).
 */
export const PH_BASEMAP_SOURCE_ID = 'openmaptiles'
export const PH_BASEMAP_STYLE_HOST = 'tiles.openfreemap.org'

export interface BasemapFragment {
  glyphs: string
  sources: Record<string, SourceSpecification>
  layers: LayerSpecification[]
  /**
   * The Black Marble raster's `raster-opacity`, the exact MIRROR of the
   * background ramp baked into `layers` above. Both come from `zoomRamp`
   * below with swapped endpoints, so the two halves of the crossfade cannot
   * drift apart the way two hand-written `interpolate` arrays in two files
   * would. The caller owns the raster LAYER (it is not part of the basemap
   * style); this module owns the HANDOFF.
   */
  rasterFadeOut: ExpressionSpecification
  /** Layer `maxzoom` for that raster — see RASTER_CUTOFF_MARGIN. */
  rasterMaxZoom: number
}

/** A linear zoom ramp between two opacity endpoints. */
function zoomRamp(
  fadeStart: number,
  fadeEnd: number,
  from: number,
  to: number,
): ExpressionSpecification {
  return ['interpolate', ['linear'], ['zoom'], fadeStart, from, fadeEnd, to]
}

/**
 * The basemap style fragment plus the globe→street handoff: the background
 * layer fades IN across [fadeStart, fadeEnd] and the returned
 * `rasterFadeOut` fades the Black Marble raster OUT across the same range.
 *
 * Why the background must ramp: MapLibre's background layer paints the whole
 * VIEWPORT (screen-space), not just the sphere — at globe zooms an opaque
 * background would cover the CSS starfield behind the transparent canvas.
 * The sphere fills the viewport well before fadeStart (screen radius ≈
 * 512·2^z/2π px), so ramping the background in with the street style never
 * flashes a visible rectangle over space.
 *
 * Every other basemap layer is minzoom-gated in the JSON (≥5 — half a zoom
 * BEFORE fadeStart, deliberately, so the vector tiles for the first visible
 * frame are already in flight when the crossfade begins), and any layer
 * appearing strictly INSIDE the fade window carries its own opacity ramp so
 * nothing hard-pops while the raster is still partly opaque. Below zoom 5
 * nothing vector renders, or costs a tile fetch, under the opaque raster.
 */
export function phBasemapFragment(
  fadeStart: number,
  fadeEnd: number,
): BasemapFragment {
  // Deep-clone before handing anything to MapLibre: the JSON import is a
  // module singleton, and Atlas creates a FRESH map on every show (Cache
  // Components hide/show) — sharing mutable sub-objects across map
  // instances would let any in-place edit leak between mounts.
  const cloned = structuredClone(style)
  const layers = cloned.layers.map((layer) =>
    layer.type === 'background'
      ? {
          ...layer,
          paint: {
            ...layer.paint,
            'background-opacity': zoomRamp(fadeStart, fadeEnd, 0, 1),
          },
        }
      : layer,
  ) as LayerSpecification[]
  return {
    glyphs: cloned.glyphs as string,
    sources: cloned.sources,
    layers,
    rasterFadeOut: zoomRamp(fadeStart, fadeEnd, 1, 0),
    rasterMaxZoom: fadeEnd + RASTER_CUTOFF_MARGIN,
  }
}
