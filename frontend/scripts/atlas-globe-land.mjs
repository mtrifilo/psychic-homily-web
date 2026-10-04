#!/usr/bin/env node
// Builds public/atlas/globe-land-110m.geojson, the land polygons the compact
// Atlas globe draws in place of the night-earth raster.
//
// Input: Natural Earth 1:110m land (public domain, no credit required:
// https://www.naturalearthdata.com/about/terms-of-use/), pinned to the
// natural-earth-vector commit the committed file was built from:
// https://raw.githubusercontent.com/nvkelso/natural-earth-vector/693f11422f4e08d2da4566b854dda53eb7c39fb3/geojson/ne_110m_land.geojson
// sha256 9e0729ee253ca7d7a5c4ae9395fb1902264c5377c52e224d13dd85010e2835d9
//
// Output: one Feature with one MultiPolygon (properties dropped, coordinates
// rounded to COORD_DECIMALS, consecutive duplicate vertices removed, rings
// that collapse below four positions dropped). 0.01 degree is about 1 km:
// under a pixel below z5.5, where the land layer is fully opaque, and under
// two pixels at z7, where it has faded out.
//
// Usage:
//   node scripts/atlas-globe-land.mjs ne_110m_land.geojson > public/atlas/globe-land-110m.geojson
import { readFileSync } from 'node:fs'

const COORD_DECIMALS = 2

const [inputPath] = process.argv.slice(2)
if (!inputPath) {
  console.error('usage: node scripts/atlas-globe-land.mjs <ne_110m_land.geojson>')
  process.exit(2)
}

const factor = 10 ** COORD_DECIMALS
const round = (n) => Math.round(n * factor) / factor

function compactRing(ring) {
  const out = []
  for (const [lng, lat] of ring) {
    const p = [round(lng), round(lat)]
    const last = out[out.length - 1]
    if (!last || last[0] !== p[0] || last[1] !== p[1]) out.push(p)
  }
  const first = out[0]
  const last = out[out.length - 1]
  if (first[0] !== last[0] || first[1] !== last[1]) out.push(first)
  return out
}

const input = JSON.parse(readFileSync(inputPath, 'utf8'))
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
