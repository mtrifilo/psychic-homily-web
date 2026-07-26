// PH-branded dark basemap (PSY-1543) — the street-level vector style the
// Atlas globe crossfades into. The style itself lives in ph-dark-basemap.json
// (a complete standalone MapLibre style, versioned in-repo; see README.md in
// this directory for provenance, regeneration, attribution, and the
// Protomaps self-host fallback). This module adapts it into the fragment
// GlobeCanvas merges into its inline style.
import type {
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

export interface BasemapFragment {
  glyphs: string
  sources: Record<string, SourceSpecification>
  layers: LayerSpecification[]
}

/**
 * The basemap style fragment, with the globe→street handoff applied: the
 * background layer fades IN across [fadeStart, fadeEnd] as the caller fades
 * the Black Marble raster OUT across the same range.
 *
 * Why the background must ramp: MapLibre's background layer paints the whole
 * VIEWPORT (screen-space), not just the sphere — at globe zooms an opaque
 * background would cover the CSS starfield behind the transparent canvas.
 * The sphere fills the viewport well before fadeStart (screen radius ≈
 * 512·2^z/2π px), so ramping the background in with the street style never
 * flashes a visible rectangle over space.
 *
 * Every other basemap layer is minzoom-gated in the JSON (≥5), so nothing
 * vector renders — or costs tile fetches — at low globe zooms where the
 * raster fully covers it.
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
            'background-opacity': [
              'interpolate',
              ['linear'],
              ['zoom'],
              fadeStart,
              0,
              fadeEnd,
              1,
            ],
          },
        }
      : layer,
  ) as LayerSpecification[]
  return { glyphs: cloned.glyphs as string, sources: cloned.sources, layers }
}
