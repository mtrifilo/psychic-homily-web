// The light Atlas street style, derived from the dark one.
//
// ph-light-basemap.json is the dark style (ph-dark-basemap.json) with every
// colour in LIGHT_PAINT swapped in and its name changed: the same layers,
// ids, sources, filters, source layers, zoom ramps, widths, opacities and
// fonts. The light values copy the light column of the palette table on the
// Figma board (Product Designs, Atlas page, row 1973:75), the palette's
// authority; the `:root` tokens in app/globals.css are anchors only (see
// basemap/README.md).
//
// deriveLightBasemap refuses a dark style the table no longer fits: a colour
// it names that the dark layer lacks, a colour the dark layer has that it does
// not name, a dark colour other than the one it pairs, or a layer on one side
// only. A regenerated or retuned dark style therefore cannot leave a layer
// dark in the light map without the derivation failing.
import { isDeepStrictEqual } from 'node:util'

export const LIGHT_STYLE_NAME = 'Psychic Homily Light'

// --background in :root: the ground, and the halo of every label drawn on it.
const PAPER = '#f4f1ea'
// The light water fill, and the halo of the labels drawn on water.
const WATER = '#dbe4ea'

/**
 * Layer id -> paint property -> [the dark file's value, the light value].
 * Only colours change. The dark value is recorded so a recolour of the dark
 * file stops the derivation until its light twin is chosen too.
 */
export const LIGHT_PAINT = {
  'ph-background': { 'background-color': ['#0d0805', PAPER] },
  'ph-landcover-green': { 'fill-color': ['#0f1309', '#e4e9d4'] },
  'ph-landcover-ice': { 'fill-color': ['#0f1114', '#eef0f1'] },
  'ph-landcover-sand': { 'fill-color': ['#130e08', '#f0e8d5'] },
  'ph-park': { 'fill-color': ['#111408', '#dfe6c9'] },
  'ph-landuse-residential': { 'fill-color': ['#110b07', '#efe9dd'] },
  'ph-waterway': { 'line-color': ['#12181f', '#c9d6df'] },
  'ph-water': { 'fill-color': ['#090c12', WATER] },
  'ph-aeroway-runway': { 'line-color': ['#191209', '#e3dccd'] },
  'ph-road-path': { 'line-color': ['#181209', '#e9e3d6'] },
  'ph-road-service': { 'line-color': ['#1b140e', '#e4ddcd'] },
  'ph-road-minor': { 'line-color': ['#201810', '#ddd5c2'] },
  'ph-road-secondary-tertiary': { 'line-color': ['#271d13', '#d3c9b1'] },
  // --border
  'ph-road-primary-trunk': { 'line-color': ['#2f2216', '#cabe9f'] },
  'ph-road-motorway': { 'line-color': ['#34251a', '#bfb18e'] },
  'ph-rail': { 'line-color': ['#1d160f', '#d6ccb6'] },
  // --card fill, --secondary outline
  'ph-building': {
    'fill-color': ['#17100b', '#faf7f0'],
    'fill-outline-color': ['#221b15', '#e8e3d6'],
  },
  // --border
  'ph-boundary-state': { 'line-color': ['#292017', '#cabe9f'] },
  'ph-boundary-country': { 'line-color': ['#3a3024', '#a89b7d'] },
  'ph-water-name': {
    'text-color': ['#4f5c6e', '#5c6f82'],
    'text-halo-color': ['#090c12', WATER],
  },
  'ph-road-name-minor': { 'text-color': ['#7c6e5e', '#8a7b69'], 'text-halo-color': ['#0d0805', PAPER] },
  // --muted-foreground
  'ph-road-name-major': { 'text-color': ['#93836f', '#6b5e4f'], 'text-halo-color': ['#0d0805', PAPER] },
  'ph-place-suburb': { 'text-color': ['#8a7b69', '#7a6c5c'], 'text-halo-color': ['#0d0805', PAPER] },
  'ph-place-village': { 'text-color': ['#ab9d8a', '#5a4f43'], 'text-halo-color': ['#0d0805', PAPER] },
  'ph-place-town': { 'text-color': ['#c4b6a1', '#3e352c'], 'text-halo-color': ['#0d0805', PAPER] },
  // --foreground
  'ph-place-city': { 'text-color': ['#eee7d9', '#1a1714'], 'text-halo-color': ['#0d0805', PAPER] },
}

// A paint property holds a colour when its name ends in -color.
const isColorProperty = (name) => name.endsWith('-color')

/**
 * The light style for `dark`, a new object; `dark` is not modified. Throws
 * when LIGHT_PAINT and the dark style's colours do not match one for one.
 */
export function deriveLightBasemap(dark) {
  const light = structuredClone(dark)
  light.name = LIGHT_STYLE_NAME
  const ids = new Set(light.layers.map((layer) => layer.id))
  for (const id of Object.keys(LIGHT_PAINT)) {
    if (!ids.has(id)) throw new Error(`LIGHT_PAINT names ${id}, which the dark style lacks`)
  }
  for (const layer of light.layers) {
    const colors = LIGHT_PAINT[layer.id]
    if (!colors) throw new Error(`${layer.id} has no entry in LIGHT_PAINT`)
    const paint = layer.paint ?? {}
    const darkColors = Object.keys(paint).filter(isColorProperty).sort()
    const named = Object.keys(colors).sort()
    if (darkColors.join() !== named.join()) {
      throw new Error(
        `${layer.id}: the dark style colours ${darkColors.join(', ') || 'nothing'}; LIGHT_PAINT names ${named.join(', ')}`,
      )
    }
    for (const [property, [darkValue]] of Object.entries(colors)) {
      if (paint[property] !== darkValue) {
        throw new Error(
          `${layer.id} ${property} is ${paint[property]} in the dark file; LIGHT_PAINT pairs ${darkValue}`,
        )
      }
    }
    layer.paint = {
      ...paint,
      ...Object.fromEntries(Object.entries(colors).map(([property, [, light]]) => [property, light])),
    }
  }
  return light
}

/**
 * The light style file's text: the dark file's text with the name and each
 * colour LIGHT_PAINT names replaced in place, so the two files keep the same
 * layout and diff line by line on those values alone. Throws unless the
 * result parses to exactly deriveLightBasemap's style.
 */
export function lightBasemapText(darkText) {
  const dark = JSON.parse(darkText)
  const light = deriveLightBasemap(dark)
  const pair = (key, value) => `${JSON.stringify(key)}: ${JSON.stringify(value)}`
  const replaceOnce = (segment, from, to, where) => {
    const at = segment.indexOf(from)
    if (at === -1 || segment.indexOf(from, at + 1) !== -1) {
      throw new Error(`${where}: expected exactly one ${from}`)
    }
    return segment.slice(0, at) + to + segment.slice(at + from.length)
  }

  // Each layer's text runs from its id to the next layer's id.
  const starts = dark.layers.map((layer) => darkText.indexOf(pair('id', layer.id)))
  if (starts.some((at, i) => at === -1 || (i > 0 && at <= starts[i - 1]))) {
    throw new Error('the dark file does not list its layer ids in order')
  }
  const head = replaceOnce(
    darkText.slice(0, starts[0]),
    pair('name', dark.name),
    pair('name', light.name),
    'style',
  )
  const layers = dark.layers.map((layer, i) => {
    let segment = darkText.slice(starts[i], starts[i + 1] ?? darkText.length)
    for (const [property, [, value]] of Object.entries(LIGHT_PAINT[layer.id])) {
      segment = replaceOnce(
        segment,
        pair(property, layer.paint[property]),
        pair(property, value),
        layer.id,
      )
    }
    return segment
  })
  const text = head + layers.join('')
  if (!isDeepStrictEqual(JSON.parse(text), light)) {
    throw new Error('the rewritten text does not parse to the derived light style')
  }
  return text
}
