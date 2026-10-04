#!/usr/bin/env node
// Builds public/atlas/globe-land-110m.geojson, the land polygons the compact
// Atlas globe draws in place of the night-earth raster.
//
// Input: Natural Earth 1:110m land, ne_110m_land.geojson, pinned and checked
// as scripts/lib/natural-earth.mjs describes.
//
// Output: one Feature with one MultiPolygon (properties dropped, coordinates
// rounded to COORD_DECIMALS, consecutive duplicate vertices removed, rings
// that collapse below four positions dropped).
//
// Usage:
//   node scripts/atlas-globe-land.mjs ne_110m_land.geojson > public/atlas/globe-land-110m.geojson
import { compactPositions, readPinned } from './lib/natural-earth.mjs'

const INPUT_SHA256 = '9e0729ee253ca7d7a5c4ae9395fb1902264c5377c52e224d13dd85010e2835d9'

const [inputPath] = process.argv.slice(2)
if (!inputPath) {
  console.error('usage: node scripts/atlas-globe-land.mjs <ne_110m_land.geojson>')
  process.exit(2)
}

function compactRing(ring) {
  const out = compactPositions(ring)
  const first = out[0]
  const last = out[out.length - 1]
  if (first[0] !== last[0] || first[1] !== last[1]) out.push(first)
  return out
}

const input = readPinned(inputPath, INPUT_SHA256)
const polygons = []
for (const feature of input.features) {
  const { type, coordinates } = feature.geometry
  const parts = type === 'Polygon' ? [coordinates] : coordinates
  for (const [outerRing, ...holeRings] of parts) {
    const outer = compactRing(outerRing)
    // An outer ring that collapsed has no area left to draw.
    if (outer.length < 4) continue
    const holes = holeRings.map(compactRing).filter((r) => r.length >= 4)
    polygons.push([outer, ...holes])
  }
}

process.stdout.write(
  JSON.stringify({
    type: 'FeatureCollection',
    features: [
      {
        type: 'Feature',
        properties: {},
        geometry: { type: 'MultiPolygon', coordinates: polygons },
      },
    ],
  }),
)
