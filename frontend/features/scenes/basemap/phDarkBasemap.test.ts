import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { validateStyleMin } from '@maplibre/maplibre-gl-style-spec'
import type { StyleSpecification } from 'maplibre-gl'
import phDarkBasemap from './ph-dark-basemap.json'
import {
  PH_BASEMAP_MIN_ZOOM,
  PH_BASEMAP_SOURCE_ID,
  PH_BASEMAP_TILE_HOST,
  phBasemapFragment,
} from './phBasemap'

/**
 * Guards for the PH dark basemap style (PSY-1543).
 *
 * The style JSON is hand-curated against the OpenMapTiles schema served by
 * OpenFreeMap (see README.md in this directory). These tests pin the
 * properties that break silently in production:
 *  - spec validity (a malformed layer aborts the whole style load — the map
 *    renders nothing, no partial fallback),
 *  - every remote host being present in the app CSP's connect-src (MapLibre
 *    fetches tiles AND glyphs via fetch(); a missing host = blank basemap
 *    with only console CSP noise),
 *  - fonts staying within OpenFreeMap's fixed glyph set (an unknown
 *    fontstack 404s per label tile → labels silently absent),
 *  - zero CARTO leftovers from the spike (ticket AC), and
 *  - the merge contract GlobeCanvas relies on (background first, all vector
 *    layers minzoom-gated, no id collisions with the scene layers).
 */

const FRONTEND_ROOT = path.resolve(__dirname, '../../..')
const STYLE_PATH = path.join(FRONTEND_ROOT, 'features/scenes/basemap/ph-dark-basemap.json')

const style = phDarkBasemap as unknown as StyleSpecification

// OpenFreeMap's glyph server serves a fixed font set; these three are the
// stacks verified available (HTTP 200 on 0-255.pbf, 2026-07-26). Space Mono
// and other PH type-direction faces are NOT served — see README.md.
const AVAILABLE_FONTS = new Set([
  'Noto Sans Regular',
  'Noto Sans Bold',
  'Noto Sans Italic',
])

// Layer ids GlobeCanvas appends after the basemap fragment — a collision
// would make the merged style invalid and abort the load.
const GLOBE_CANVAS_LAYER_IDS = ['earth', 'scene-rings', 'scene-dots']
const GLOBE_CANVAS_SOURCE_IDS = ['nightEarth', 'scenes', 'scene-rings']

function collectRemoteUrls(s: StyleSpecification): string[] {
  const urls: string[] = []
  if (typeof s.glyphs === 'string') urls.push(s.glyphs)
  if (typeof s.sprite === 'string') urls.push(s.sprite)
  for (const source of Object.values(s.sources)) {
    if ('url' in source && typeof source.url === 'string') urls.push(source.url)
    if ('tiles' in source && Array.isArray(source.tiles)) {
      urls.push(...source.tiles)
    }
  }
  return urls.filter((u) => u.startsWith('http'))
}

describe('ph-dark-basemap.json', () => {
  it('validates against the MapLibre style spec', () => {
    const errors = validateStyleMin(style)
    expect(errors).toEqual([])
  })

  it('contains zero CARTO references (PSY-1543 AC — the spike borrowed dark-matter)', () => {
    const raw = readFileSync(STYLE_PATH, 'utf8')
    expect(raw.toLowerCase()).not.toContain('carto')
    expect(raw.toLowerCase()).not.toContain('dark-matter')
  })

  it('only references remote hosts allowlisted in the CSP connect-src', () => {
    const remoteHosts = new Set(
      collectRemoteUrls(style).map((u) => new URL(u).host),
    )
    expect(remoteHosts.size).toBeGreaterThan(0)
    const nextConfig = readFileSync(
      path.join(FRONTEND_ROOT, 'next.config.ts'),
      'utf8',
    )
    const connectSrc = nextConfig
      .split('\n')
      .find((line) => line.includes("connect-src 'self'"))
    expect(connectSrc).toBeDefined()
    for (const host of remoteHosts) {
      expect(connectSrc, `CSP connect-src is missing https://${host}`).toContain(
        `https://${host}`,
      )
    }
  })

  it('matches the source id and host the failure signal filters on (PSY-1568)', () => {
    // basemapTelemetry reports a failure of THIS source and ignores every
    // other MapLibre error. A regenerated style that renames the source or
    // moves hosts would leave that filter matching nothing — the exact silent
    // degradation the signal exists to catch — so it fails here instead.
    expect(Object.keys(style.sources)).toEqual([PH_BASEMAP_SOURCE_ID])
    for (const url of collectRemoteUrls(style)) {
      expect(new URL(url).hostname).toBe(PH_BASEMAP_TILE_HOST)
    }
  })

  it('uses only fontstacks the OpenFreeMap glyph server actually serves', () => {
    const fonts = new Set<string>()
    for (const layer of style.layers) {
      const textFont =
        layer.type === 'symbol' ? layer.layout?.['text-font'] : undefined
      if (Array.isArray(textFont)) {
        for (const f of textFont) if (typeof f === 'string') fonts.add(f)
      }
    }
    expect(fonts.size).toBeGreaterThan(0)
    for (const font of fonts) {
      expect(AVAILABLE_FONTS.has(font), `unknown fontstack: ${font}`).toBe(true)
    }
  })

  it('declares no sprite and uses no icons (no sprite host is CSP-allowlisted)', () => {
    expect(style.sprite).toBeUndefined()
    for (const layer of style.layers) {
      if (layer.type === 'symbol') {
        expect(
          layer.layout && 'icon-image' in layer.layout,
          `${layer.id} uses icon-image`,
        ).toBeFalsy()
      }
    }
  })

  it('keeps every layer/source id clear of the GlobeCanvas scene layers', () => {
    const ids = style.layers.map((l) => l.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const id of ids) {
      expect(id.startsWith('ph-'), `${id} must be ph- prefixed`).toBe(true)
      expect(GLOBE_CANVAS_LAYER_IDS).not.toContain(id)
    }
    for (const sourceId of Object.keys(style.sources)) {
      expect(GLOBE_CANVAS_SOURCE_IDS).not.toContain(sourceId)
    }
  })

  it('gates every non-background layer to PH_BASEMAP_MIN_ZOOM (nothing renders or fetches at globe zooms)', () => {
    for (const layer of style.layers) {
      if (layer.type === 'background') continue
      expect(
        layer.minzoom,
        `${layer.id} needs a minzoom >= ${PH_BASEMAP_MIN_ZOOM}`,
      ).toBeGreaterThanOrEqual(PH_BASEMAP_MIN_ZOOM)
    }
  })

  // The crossfade range GlobeCanvas drives the handoff over. Any layer that
  // switches on strictly INSIDE it appears while the Black Marble raster is
  // still partly opaque, and MapLibre's `minzoom` is a hard cutoff with no
  // fade of its own — so such a layer must ramp its own opacity or it hard-
  // pops mid-transition (the "no pop" acceptance criterion).
  const FADE_START = 5.5
  const FADE_END = 7
  const OPACITY_PROP: Record<string, string> = {
    line: 'line-opacity',
    fill: 'fill-opacity',
    symbol: 'text-opacity',
    raster: 'raster-opacity',
    background: 'background-opacity',
  }

  it('fades in (never hard-pops) every layer that appears mid-crossfade', () => {
    for (const layer of style.layers) {
      const minzoom = layer.minzoom ?? 0
      if (minzoom <= FADE_START || minzoom >= FADE_END) continue
      const prop = OPACITY_PROP[layer.type]
      const paint = layer.paint as Record<string, unknown> | undefined
      const opacity = prop ? paint?.[prop] : undefined
      expect(
        Array.isArray(opacity) && opacity[0] === 'interpolate',
        `${layer.id} switches on at zoom ${minzoom}, inside the [${FADE_START}, ${FADE_END}] crossfade, so it needs a zoom-interpolated ${prop} instead of a hard minzoom cutoff`,
      ).toBe(true)
      // Resolve at the same zoom as the rest of the descent, not later.
      const stops = opacity as unknown[]
      expect(stops[stops.length - 2]).toBe(FADE_END)
      expect(stops[stops.length - 1]).toBe(1)
    }
  })

  // The style is anchored to the app's `.dark` tokens (README.md "Palette").
  // Nothing at runtime reads those tokens — the style JSON is a hand-copied
  // snapshot of them — so a future dark-theme repaint would leave the map
  // silently stale with CI green. These are the anchors the style actually
  // paints with; water is deliberately NOT in the list (a documented
  // departure from the token palette, not a copy of it), and
  // --muted-foreground is checked separately below because it is used by the
  // attribution CSS rather than by the style.
  const PALETTE_ANCHORS: Record<string, string> = {
    '--background': '#0d0805',
    '--foreground': '#eee7d9',
    '--card': '#17100b',
    '--border': '#221b15',
  }

  it('keeps its palette anchored to the .dark tokens in globals.css', () => {
    const css = readFileSync(path.join(FRONTEND_ROOT, 'app/globals.css'), 'utf8')
    const darkBlock = css.slice(css.indexOf('.dark {'))
    const raw = readFileSync(STYLE_PATH, 'utf8').toLowerCase()
    for (const [token, hex] of Object.entries(PALETTE_ANCHORS)) {
      expect(
        darkBlock,
        `${token} moved in globals.css — re-derive the basemap palette (see basemap/README.md) and update this guard`,
      ).toContain(`${token}: ${hex};`)
      expect(raw, `${hex} (${token}) is no longer used by the style`).toContain(
        hex,
      )
    }
  })

  it('keeps the attribution restyle anchored to the same dark tokens', () => {
    const css = readFileSync(path.join(FRONTEND_ROOT, 'app/globals.css'), 'utf8')
    const darkBlock = css.slice(css.indexOf('.dark {'))
    // The map's chrome is deliberately static (the Atlas surface stays dark
    // in both app themes), so these hexes are literals in the stylesheet and
    // drift from the tokens they were picked from just as silently.
    expect(darkBlock).toContain('--muted-foreground: #9c8c7c;')
    const attrib = css.slice(css.indexOf('.maplibregl-ctrl-attrib'))
    expect(attrib).toContain('#9c8c7c')
  })

  it('credits OpenStreetMap on the vector source (ODbL requirement)', () => {
    const omt = style.sources.openmaptiles
    expect(omt).toBeDefined()
    expect('attribution' in omt && omt.attribution).toContain('OpenStreetMap')
  })
})

describe('phBasemapFragment', () => {
  it('starts with the background layer so the GlobeCanvas spread keeps it at the bottom', () => {
    const { layers } = phBasemapFragment(5.5, 7)
    expect(layers[0]?.type).toBe('background')
  })

  it('ramps background-opacity across the requested fade range', () => {
    const { layers } = phBasemapFragment(5.5, 7)
    const background = layers[0]
    if (background.type !== 'background') throw new Error('unreachable')
    expect(background.paint?.['background-opacity']).toEqual([
      'interpolate',
      ['linear'],
      ['zoom'],
      5.5,
      0,
      7,
      1,
    ])
  })

  it('fades the raster out exactly as it fades the background in (the crossfade is symmetric)', () => {
    const { layers, rasterFadeOut } = phBasemapFragment(5.5, 7)
    const background = layers[0]
    if (background.type !== 'background') throw new Error('unreachable')
    // Same interpolation, same stops, swapped endpoints. An asymmetric pair
    // is what produces the two failure modes this handoff exists to avoid:
    // a black void (both near 0) or a double-drawn earth (both near 1).
    expect(rasterFadeOut).toEqual(['interpolate', ['linear'], ['zoom'], 5.5, 1, 7, 0])
    const [, , , bgStart, bgFrom, bgEnd, bgTo] = background.paint?.[
      'background-opacity'
    ] as unknown as [string, string[], string[], number, number, number, number]
    const [, , , rStart, rFrom, rEnd, rTo] = rasterFadeOut as unknown as [
      string,
      string[],
      string[],
      number,
      number,
      number,
      number,
    ]
    expect([rStart, rEnd]).toEqual([bgStart, bgEnd])
    expect(rFrom + bgFrom).toBe(1)
    expect(rTo + bgTo).toBe(1)
  })

  it('unmounts the raster only past the point the fade has reached zero', () => {
    const { rasterMaxZoom } = phBasemapFragment(5.5, 7)
    expect(rasterMaxZoom).toBeGreaterThan(7)
  })

  it('passes glyphs and sources through from the style JSON', () => {
    const fragment = phBasemapFragment(5.5, 7)
    expect(fragment.glyphs).toBe(style.glyphs)
    expect(Object.keys(fragment.sources)).toEqual(['openmaptiles'])
  })

  it('leaves non-background layers untouched', () => {
    const { layers } = phBasemapFragment(5.5, 7)
    expect(layers.slice(1)).toEqual(style.layers.slice(1))
  })
})
