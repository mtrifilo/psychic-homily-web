// The light Atlas street style, derived from the dark one.
//
// ph-light-basemap.json is the dark style (ph-dark-basemap.json) with every
// colour in LIGHT_PAINT swapped in and its name changed: the same layers,
// ids, sources, filters, source layers, zoom ramps, widths, opacities and
// fonts. The values are the light column of the palette table on the Figma
// board (Product Designs, Atlas page, PSY-1567 row), derived from the `:root`
// tokens in app/globals.css as basemap/README.md describes.
//
// deriveLightBasemap refuses a dark style the table no longer fits: a colour
// it names that the dark layer lacks, a colour the dark layer has that it does
// not name, or a layer on one side only. A regenerated dark style therefore
// cannot leave a layer dark in the light map without the derivation failing.
import { isDeepStrictEqual } from 'node:util'

export const LIGHT_STYLE_NAME = 'Psychic Homily Light'

// --background in :root: the ground, and the halo of every label drawn on it.
const PAPER = '#f4f1ea'
// The light water fill, and the halo of the labels drawn on water.
const WATER = '#dbe4ea'

/** Layer id -> paint property -> light value. Only colours change. */
export const LIGHT_PAINT = {
  'ph-background': { 'background-color': PAPER },
  'ph-landcover-green': { 'fill-color': '#e4e9d4' },
  'ph-landcover-ice': { 'fill-color': '#eef0f1' },
  'ph-landcover-sand': { 'fill-color': '#f0e8d5' },
  'ph-park': { 'fill-color': '#dfe6c9' },
  'ph-landuse-residential': { 'fill-color': '#efe9dd' },
  'ph-waterway': { 'line-color': '#c9d6df' },
  'ph-water': { 'fill-color': WATER },
  'ph-aeroway-runway': { 'line-color': '#e3dccd' },
  'ph-road-path': { 'line-color': '#e9e3d6' },
  'ph-road-service': { 'line-color': '#e4ddcd' },
  'ph-road-minor': { 'line-color': '#ddd5c2' },
  'ph-road-secondary-tertiary': { 'line-color': '#d3c9b1' },
  // --border
  'ph-road-primary-trunk': { 'line-color': '#cabe9f' },
  'ph-road-motorway': { 'line-color': '#bfb18e' },
  'ph-rail': { 'line-color': '#d6ccb6' },
  // --card fill, --secondary outline
  'ph-building': { 'fill-color': '#faf7f0', 'fill-outline-color': '#e8e3d6' },
  // --border
  'ph-boundary-state': { 'line-color': '#cabe9f' },
  'ph-boundary-country': { 'line-color': '#a89b7d' },
  'ph-water-name': { 'text-color': '#5c6f82', 'text-halo-color': WATER },
  'ph-road-name-minor': { 'text-color': '#8a7b69', 'text-halo-color': PAPER },
  // --muted-foreground
  'ph-road-name-major': { 'text-color': '#6b5e4f', 'text-halo-color': PAPER },
  'ph-place-suburb': { 'text-color': '#7a6c5c', 'text-halo-color': PAPER },
  'ph-place-village': { 'text-color': '#5a4f43', 'text-halo-color': PAPER },
  'ph-place-town': { 'text-color': '#3e352c', 'text-halo-color': PAPER },
  // --foreground
  'ph-place-city': { 'text-color': '#1a1714', 'text-halo-color': PAPER },
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
    layer.paint = { ...paint, ...colors }
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
    for (const [property, value] of Object.entries(LIGHT_PAINT[layer.id])) {
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
