// PH-branded street basemap: the street-level vector style the Atlas globe
// crossfades into, in two variants. The dark style lives in
// ph-dark-basemap.json and the light one in ph-light-basemap.json, which
// scripts/atlas-light-basemap.mjs writes from the dark file (only colours
// differ). Each is a complete standalone MapLibre style, versioned in-repo;
// see README.md in this directory for provenance, regeneration, attribution,
// and the Protomaps self-host fallback. This module adapts one of them into
// the fragment a map merges into its inline style.
import type {
  ExpressionSpecification,
  LayerSpecification,
  SourceSpecification,
  StyleSpecification,
} from 'maplibre-gl'
import phDarkBasemap from './ph-dark-basemap.json'
import phLightBasemap from './ph-light-basemap.json'

/** Which street style a map draws. */
export type BasemapVariant = 'dark' | 'light'

// resolveJsonModule types JSON literals as plain strings, not the style-spec
// literal unions — the cast is unavoidable. phBasemap.test.ts validates both
// files against @maplibre/maplibre-gl-style-spec so the cast can't hide a
// malformed style.
const STYLES: Readonly<Record<BasemapVariant, StyleSpecification>> = {
  dark: phDarkBasemap as unknown as StyleSpecification,
  light: phLightBasemap as unknown as StyleSpecification,
}

/** A style's ground: the colour of its background layer. */
function groundColor(style: StyleSpecification): string {
  const background = style.layers.find((layer) => layer.type === 'background')
  const color =
    background?.type === 'background'
      ? background.paint?.['background-color']
      : undefined
  if (typeof color !== 'string') {
    throw new Error(`${style.name} has no background colour`)
  }
  return color
}

// How far past fadeEnd the raster layer stays mounted. The fade reaches 0
// exactly AT fadeEnd; a layer `maxzoom` is exclusive, so cutting off at
// fadeEnd itself would drop the layer on the same frame the ramp lands and
// risk a one-frame seam on a slow tile. A hair past it unmounts the raster
// (no more GIBS fetches, no more compositing) only once it is provably
// invisible.
const RASTER_CUTOFF_MARGIN = 0.1

/**
 * The zoom at which the style's vector layers switch on — every non-background
 * layer in both style files is gated at or above this. Deliberately half a
 * zoom BELOW the crossfade start so the first street tiles are already in
 * flight (and drawn, under the still-opaque raster) when the fade begins.
 * Below it nothing vector renders and no vector tile is fetched.
 *
 * Exported so the callers that must agree with it — the atmosphere ramp in
 * GlobeCanvas and the minzoom guard in phBasemap.test.ts — derive it
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
 * placeholder. Instead phBasemap.test.ts asserts both JSON files still match,
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

/** A linear zoom ramp between two opacities, or two colours. */
function zoomRamp(
  fadeStart: number,
  fadeEnd: number,
  from: number | string,
  to: number | string,
): ExpressionSpecification {
  return ['interpolate', ['linear'], ['zoom'], fadeStart, from, fadeEnd, to]
}

/**
 * The basemap style fragment plus the globe→street handoff: the background
 * layer fades IN across [fadeStart, fadeEnd] and the returned
 * `rasterFadeOut` fades the Black Marble raster OUT across the same range.
 *
 * Why the background ramps: it is the street style's ground, and the ramp is
 * the mirror of the globe surface's fade-out, so the ground is the globe
 * surface at globe zooms and the street background at street zooms, with
 * complementary opacities across the handoff. On the globe projection MapLibre
 * draws a background layer on the sphere only, not the whole viewport, so
 * space around the globe stays transparent for the CSS starfield either way.
 *
 * Every other basemap layer is minzoom-gated in the JSON (≥5 — half a zoom
 * BEFORE fadeStart, deliberately, so the vector tiles for the first visible
 * frame are already in flight when the crossfade begins), and any layer
 * appearing strictly INSIDE the fade window carries its own opacity ramp so
 * nothing hard-pops while the raster is still partly opaque. Below zoom 5
 * nothing vector renders, or costs a tile fetch, under the opaque raster.
 *
 * The light variant's ground also ramps its COLOUR across the same range,
 * from the dark style's ground at fadeStart to paper at fadeEnd: the night
 * globe dissolves into a ground that brightens with it, so no paper shows
 * through a still-dark raster and nothing switches at the end of the fade.
 * The dark variant's ground colour is constant.
 */
export function phBasemapFragment(
  fadeStart: number,
  fadeEnd: number,
  variant: BasemapVariant,
): BasemapFragment {
  // Deep-clone before handing anything to MapLibre: the JSON import is a
  // module singleton, and Atlas creates a FRESH map on every show (Cache
  // Components hide/show) — sharing mutable sub-objects across map
  // instances would let any in-place edit leak between mounts.
  const cloned = structuredClone(STYLES[variant])
  const groundRamp =
    variant === 'light'
      ? {
          'background-color': zoomRamp(
            fadeStart,
            fadeEnd,
            groundColor(STYLES.dark),
            groundColor(cloned),
          ),
        }
      : {}
  const layers = cloned.layers.map((layer) =>
    layer.type === 'background'
      ? {
          ...layer,
          paint: {
            ...layer.paint,
            ...groundRamp,
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
