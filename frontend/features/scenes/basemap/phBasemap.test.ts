import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { validateStyleMin } from '@maplibre/maplibre-gl-style-spec'
import type { LayerSpecification, StyleSpecification } from 'maplibre-gl'
import phDarkBasemap from './ph-dark-basemap.json'
import phLightBasemap from './ph-light-basemap.json'
import { NIGHT_EARTH_SOURCE_ID } from './nightEarthRaster'
import {
  GLOBE_COUNTRY_LINES_LAYER_ID,
  GLOBE_COUNTRY_LINES_SOURCE_ID,
  GLOBE_LAND_LAYER_ID,
  GLOBE_LAND_SOURCE_ID,
  GLOBE_OCEAN_LAYER_ID,
  GLOBE_STATE_LINES_LAYER_ID,
  GLOBE_STATE_LINES_SOURCE_ID,
  NIGHT_EARTH_LAYER_ID,
} from './globeSurface'
import {
  type BasemapVariant,
  PH_BASEMAP_MIN_ZOOM,
  PH_BASEMAP_SOURCE_ID,
  PH_BASEMAP_STYLE_HOST,
  phBasemapFragment,
} from './phBasemap'
import {
  LIGHT_PAINT,
  LIGHT_STYLE_NAME,
  deriveLightBasemap,
  lightBasemapText,
} from '../../../scripts/lib/ph-light-basemap.mjs'

/**
 * Guards for the PH street basemap styles, dark and light.
 *
 * Both style JSONs target the OpenMapTiles schema served by OpenFreeMap (see
 * README.md in this directory); the light file is written from the dark one
 * by scripts/atlas-light-basemap.mjs. These tests pin, for each file, the
 * properties that break silently in production:
 *  - spec validity (a malformed layer aborts the whole style load — the map
 *    renders nothing, no partial fallback),
 *  - every remote host being present in the app CSP's connect-src (MapLibre
 *    fetches tiles AND glyphs via fetch(); a missing host = blank basemap
 *    with only console CSP noise),
 *  - fonts staying within OpenFreeMap's fixed glyph set (an unknown
 *    fontstack 404s per label tile → labels silently absent),
 *  - zero CARTO leftovers from the spike (ticket AC),
 *  - the merge contract GlobeCanvas relies on (background first, all vector
 *    layers minzoom-gated, no id collisions with the scene layers), and
 *  - the palette anchors against the theme's tokens in globals.css.
 * Then that the two files differ in colours alone, and that the committed
 * light file is what the generator writes today.
 */

const FRONTEND_ROOT = path.resolve(__dirname, '../../..')
const BASEMAP_DIR = path.join(FRONTEND_ROOT, 'features/scenes/basemap')
const GLOBALS_CSS = readFileSync(path.join(FRONTEND_ROOT, 'app/globals.css'), 'utf8')

// The theme blocks of globals.css: `:root` holds the light tokens and
// `.dark` overrides them.
const ROOT_BLOCK = GLOBALS_CSS.slice(
  GLOBALS_CSS.indexOf(':root {'),
  GLOBALS_CSS.indexOf('.dark {'),
)
const DARK_BLOCK = GLOBALS_CSS.slice(GLOBALS_CSS.indexOf('.dark {'))

interface VariantCase {
  file: string
  style: StyleSpecification
  tokenBlock: { name: string; css: string }
  /**
   * The theme tokens the style paints with, each a hand-copied snapshot of
   * globals.css. Water is deliberately NOT in either list: it is a documented
   * departure from the token palette, not a copy of it.
   */
  anchors: Record<string, string>
}

const VARIANTS: Record<BasemapVariant, VariantCase> = {
  dark: {
    file: 'ph-dark-basemap.json',
    style: phDarkBasemap as unknown as StyleSpecification,
    tokenBlock: { name: '.dark', css: DARK_BLOCK },
    anchors: {
      '--background': '#0d0805',
      '--foreground': '#eee7d9',
      '--card': '#17100b',
      '--border': '#221b15',
    },
  },
  light: {
    file: 'ph-light-basemap.json',
    style: phLightBasemap as unknown as StyleSpecification,
    tokenBlock: { name: ':root', css: ROOT_BLOCK },
    anchors: {
      '--background': '#f4f1ea',
      '--foreground': '#1a1714',
      '--card': '#faf7f0',
      '--secondary': '#e8e3d6',
      '--border': '#cabe9f',
      '--muted-foreground': '#6b5e4f',
    },
  },
}
const VARIANT_NAMES = Object.keys(VARIANTS) as BasemapVariant[]

const rawStyle = (variant: BasemapVariant) =>
  readFileSync(path.join(BASEMAP_DIR, VARIANTS[variant].file), 'utf8')

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
const GLOBE_CANVAS_LAYER_IDS = [
  NIGHT_EARTH_LAYER_ID,
  GLOBE_OCEAN_LAYER_ID,
  GLOBE_LAND_LAYER_ID,
  GLOBE_STATE_LINES_LAYER_ID,
  GLOBE_COUNTRY_LINES_LAYER_ID,
  'scene-rings',
  'scene-dots',
]
const GLOBE_CANVAS_SOURCE_IDS = [
  NIGHT_EARTH_SOURCE_ID,
  GLOBE_LAND_SOURCE_ID,
  GLOBE_STATE_LINES_SOURCE_ID,
  GLOBE_COUNTRY_LINES_SOURCE_ID,
  'scenes',
  'scene-rings',
]

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

describe.each(VARIANT_NAMES)('ph-%s-basemap.json', (variant) => {
  const { style, tokenBlock, anchors } = VARIANTS[variant]

  it('validates against the MapLibre style spec', () => {
    const errors = validateStyleMin(style)
    expect(errors).toEqual([])
  })

  it('contains zero CARTO references (PSY-1543 AC — the spike borrowed dark-matter)', () => {
    const raw = rawStyle(variant).toLowerCase()
    expect(raw).not.toContain('carto')
    expect(raw).not.toContain('dark-matter')
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

  it('matches the source id the failure signal filters on (PSY-1568)', () => {
    // basemapTelemetry reports a failure of THIS source and ignores every
    // other MapLibre error. A regenerated style that renames the source would
    // leave that filter matching nothing — the exact silent degradation the
    // signal exists to catch — so it fails here instead.
    expect(Object.keys(style.sources)).toEqual([PH_BASEMAP_SOURCE_ID])

    // Only the hosts written IN THIS FILE are pinned: the source is a TileJSON
    // endpoint, so the real tile URLs come from its runtime response and no
    // test here can see them. See PH_BASEMAP_STYLE_HOST's doc comment.
    for (const url of collectRemoteUrls(style)) {
      expect(new URL(url).hostname).toBe(PH_BASEMAP_STYLE_HOST)
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

  // The style is anchored to the theme's tokens (README.md "Palette").
  // Nothing at runtime reads those tokens — the style JSON is a hand-copied
  // snapshot of them — so a future repaint of the theme would leave the map
  // silently stale with CI green.
  it(`keeps its palette anchored to the ${tokenBlock.name} tokens in globals.css`, () => {
    const raw = rawStyle(variant).toLowerCase()
    for (const [token, hex] of Object.entries(anchors)) {
      expect(
        tokenBlock.css,
        `${token} moved in globals.css — re-derive the basemap palette (see basemap/README.md) and update this guard`,
      ).toContain(`${token}: ${hex};`)
      expect(raw, `${hex} (${token}) is no longer used by the style`).toContain(
        hex,
      )
    }
  })

  it('credits OpenStreetMap on the vector source (ODbL requirement)', () => {
    const omt = style.sources.openmaptiles
    expect(omt).toBeDefined()
    expect('attribution' in omt && omt.attribution).toContain('OpenStreetMap')
  })
})

describe('the light style against the dark one', () => {
  const dark = VARIANTS.dark.style
  const light = VARIANTS.light.style
  const withoutPaint = (s: StyleSpecification) => ({
    ...s,
    name: undefined,
    layers: s.layers.map((layer) => ({ ...layer, paint: undefined })),
  })
  const paintOf = (layer: LayerSpecification) =>
    (layer.paint ?? {}) as Record<string, unknown>

  it('differs only in paint and name: same layers, ids, sources, filters, zooms and fonts', () => {
    expect(withoutPaint(light)).toEqual(withoutPaint(dark))
  })

  it('differs in paint only in colours, and changes every colour', () => {
    dark.layers.forEach((darkLayer, i) => {
      const darkPaint = paintOf(darkLayer)
      const lightPaint = paintOf(light.layers[i])
      expect(Object.keys(lightPaint)).toEqual(Object.keys(darkPaint))
      for (const [property, value] of Object.entries(darkPaint)) {
        if (property.endsWith('-color')) {
          expect(lightPaint[property], `${darkLayer.id} ${property}`).not.toEqual(value)
        } else {
          expect(lightPaint[property], `${darkLayer.id} ${property}`).toEqual(value)
        }
      }
    })
  })

  it('is the file the generator writes from the dark file today', () => {
    expect(rawStyle('light')).toBe(lightBasemapText(rawStyle('dark')))
    expect(light).toEqual(deriveLightBasemap(phDarkBasemap))
    expect(light.name).toBe(LIGHT_STYLE_NAME)
  })

  it('halos every label in the ground it sits on: paper, or water for water names', () => {
    for (const layer of light.layers) {
      if (layer.type !== 'symbol') continue
      const ground = layer.id === 'ph-water-name' ? LIGHT_PAINT['ph-water']['fill-color'] : '#f4f1ea'
      expect(paintOf(layer)['text-halo-color'], layer.id).toBe(ground)
    }
  })
})

describe('the light palette derivation', () => {
  it('refuses a dark style with a coloured layer the palette does not name', () => {
    const dark = structuredClone(phDarkBasemap) as unknown as StyleSpecification
    dark.layers.push({
      id: 'ph-new-layer',
      type: 'line',
      source: 'openmaptiles',
      'source-layer': 'transportation',
      minzoom: 12,
      paint: { 'line-color': '#123456' },
    })
    expect(() => deriveLightBasemap(dark)).toThrow(/ph-new-layer has no entry/)
  })

  it('refuses a dark layer whose colours no longer match the palette entry', () => {
    const dark = structuredClone(phDarkBasemap) as unknown as StyleSpecification
    const building = dark.layers.find((l) => l.id === 'ph-building')
    if (building?.type !== 'fill') throw new Error('unreachable')
    delete building.paint?.['fill-outline-color']
    expect(() => deriveLightBasemap(dark)).toThrow(/ph-building/)
  })

  it('refuses a palette entry for a layer the dark style dropped', () => {
    const dark = structuredClone(phDarkBasemap) as unknown as StyleSpecification
    dark.layers = dark.layers.filter((l) => l.id !== 'ph-rail')
    expect(() => deriveLightBasemap(dark)).toThrow(/names ph-rail/)
  })
})

describe('the map attribution restyle', () => {
  const rule = (selector: string) => {
    const at = GLOBALS_CSS.indexOf(`${selector} {`)
    expect(at, `${selector} rule`).toBeGreaterThan(-1)
    return GLOBALS_CSS.slice(at, GLOBALS_CSS.indexOf('}', at))
  }

  // The pill follows the theme through the tokens, so a theme repaint
  // carries it along; a literal hex here would freeze it in one theme.
  it('paints the credit in --muted-foreground on --background, both from the theme', () => {
    const pill = rule('.maplibregl-map .maplibregl-ctrl.maplibregl-ctrl-attrib')
    expect(pill).toContain('var(--background)')
    expect(pill).toContain('color: var(--muted-foreground);')
    const link = rule('.maplibregl-map .maplibregl-ctrl-attrib a')
    const hover = rule('.maplibregl-map .maplibregl-ctrl-attrib a:hover')
    for (const css of [pill, link, hover]) {
      expect(css, 'a literal colour in the attribution restyle').not.toMatch(/#[0-9a-f]{3,8}\b|rgba?\(/i)
    }
  })
})

describe.each(VARIANT_NAMES)('phBasemapFragment, %s', (variant) => {
  const { style } = VARIANTS[variant]
  const fragment = () => phBasemapFragment(FADE_START, FADE_END, variant)
  const backgroundPaint = () => {
    const background = fragment().layers[0]
    if (background.type !== 'background') throw new Error('unreachable')
    return background.paint ?? {}
  }

  it('starts with the background layer so the GlobeCanvas spread keeps it at the bottom', () => {
    expect(fragment().layers[0]?.type).toBe('background')
  })

  it('ramps background-opacity across the requested fade range', () => {
    expect(backgroundPaint()['background-opacity']).toEqual([
      'interpolate',
      ['linear'],
      ['zoom'],
      FADE_START,
      0,
      FADE_END,
      1,
    ])
  })

  it('fades the raster out exactly as it fades the background in (the crossfade is symmetric)', () => {
    const { rasterFadeOut } = fragment()
    // Same interpolation, same stops, swapped endpoints. An asymmetric pair
    // is what produces the two failure modes this handoff exists to avoid:
    // a black void (both near 0) or a double-drawn earth (both near 1).
    expect(rasterFadeOut).toEqual(['interpolate', ['linear'], ['zoom'], FADE_START, 1, FADE_END, 0])
    const [, , , bgStart, bgFrom, bgEnd, bgTo] = backgroundPaint()[
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
    expect(fragment().rasterMaxZoom).toBeGreaterThan(FADE_END)
  })

  it('passes glyphs and sources through from the style JSON', () => {
    const { glyphs, sources } = fragment()
    expect(glyphs).toBe(style.glyphs)
    expect(Object.keys(sources)).toEqual(['openmaptiles'])
  })

  it('leaves non-background layers untouched', () => {
    expect(fragment().layers.slice(1)).toEqual(style.layers.slice(1))
  })

  it('hands every map its own copy of the style', () => {
    const first = fragment()
    const second = fragment()
    expect(first.layers[1]).not.toBe(second.layers[1])
    expect(first.sources.openmaptiles).not.toBe(second.sources.openmaptiles)
  })
})

describe('phBasemapFragment ground colour', () => {
  it('keeps the dark ground one colour at every zoom, as the style file has it', () => {
    const background = phBasemapFragment(FADE_START, FADE_END, 'dark').layers[0]
    if (background.type !== 'background') throw new Error('unreachable')
    expect(background.paint).toEqual({
      'background-color': '#0d0805',
      'background-opacity': ['interpolate', ['linear'], ['zoom'], FADE_START, 0, FADE_END, 1],
    })
  })

  it('ramps the light ground from the dark ground to paper across the fade range', () => {
    const background = phBasemapFragment(FADE_START, FADE_END, 'light').layers[0]
    if (background.type !== 'background') throw new Error('unreachable')
    expect(background.paint?.['background-color']).toEqual([
      'interpolate',
      ['linear'],
      ['zoom'],
      FADE_START,
      '#0d0805',
      FADE_END,
      '#f4f1ea',
    ])
  })
})
