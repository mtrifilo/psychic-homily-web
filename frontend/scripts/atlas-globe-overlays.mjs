#!/usr/bin/env node
// Builds the compact Atlas globe's three overlay files in public/atlas/:
//
//   globe-country-lines-110m.geojson  country boundary lines (1:110m)
//   globe-state-lines-50m.geojson     state and province lines (1:50m)
//   globe-places-110m.geojson         populated places (1:110m), ranked
//
// Inputs: Natural Earth (public domain, no credit required:
// https://www.naturalearthdata.com/about/terms-of-use/), pinned to the same
// natural-earth-vector commit as scripts/atlas-globe-land.mjs:
// https://raw.githubusercontent.com/nvkelso/natural-earth-vector/693f11422f4e08d2da4566b854dda53eb7c39fb3/geojson/<name>.geojson
// Each input's sha256 is checked before anything is written.
//
// Lines: one Feature with one MultiLineString per file (properties dropped,
// coordinates rounded to COORD_DECIMALS, consecutive duplicate vertices
// removed, lines that collapse below two positions dropped).
//
// Places: one Point Feature per place with two properties, `name` and
// `rank`, written in rank order. Rank 0 is the place to label first: Natural
// Earth's `min_zoom` (the zoom its cartographers would first label the place
// at) ascending, then national capitals (`adm0cap`) before other places, then
// `pop_max` descending, then name for a stable order.
//
// Usage:
//   node scripts/atlas-globe-overlays.mjs <dir holding the three inputs> public/atlas
import { createHash } from 'node:crypto'
import { readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const COORD_DECIMALS = 2

const INPUTS = {
  countryLines: {
    file: 'ne_110m_admin_0_boundary_lines_land.geojson',
    sha256: 'a40ca2c54c19b63db803cdc4f24532d586083b160906cef35fd89bd3c7585302',
    output: 'globe-country-lines-110m.geojson',
  },
  stateLines: {
    file: 'ne_50m_admin_1_states_provinces_lines.geojson',
    sha256: 'e2bd3674134d6660337c1e02939cd910a925280b68c1adc3f92d1a556ea1273b',
    output: 'globe-state-lines-50m.geojson',
  },
  places: {
    file: 'ne_110m_populated_places_simple.geojson',
    sha256: 'd1c9602aed1860f3dcb7c75b5a269e73c0dd2b6f8a94f85ecc747f4017e5f928',
    output: 'globe-places-110m.geojson',
  },
}

const [inputDir, outputDir] = process.argv.slice(2)
if (!inputDir || !outputDir) {
  console.error('usage: node scripts/atlas-globe-overlays.mjs <input dir> <output dir>')
  process.exit(2)
}

function readPinned({ file, sha256 }) {
  const raw = readFileSync(path.join(inputDir, file))
  const actual = createHash('sha256').update(raw).digest('hex')
  if (actual !== sha256) {
    console.error(`${file}: sha256 ${actual} does not match the pinned ${sha256}`)
    process.exit(1)
  }
  return JSON.parse(raw.toString('utf8'))
}

const factor = 10 ** COORD_DECIMALS
const round = (n) => Math.round(n * factor) / factor

function compactLine(line) {
  const out = []
  for (const [lng, lat] of line) {
    const p = [round(lng), round(lat)]
    const last = out[out.length - 1]
    if (!last || last[0] !== p[0] || last[1] !== p[1]) out.push(p)
  }
  return out
}

function linesCollection(input) {
  const lines = []
  for (const feature of input.features) {
    const { type, coordinates } = feature.geometry
    const parts = type === 'LineString' ? [coordinates] : coordinates
    for (const part of parts) {
      const line = compactLine(part)
      if (line.length >= 2) lines.push(line)
    }
  }
  return {
    type: 'FeatureCollection',
    features: [
      {
        type: 'Feature',
        properties: {},
        geometry: { type: 'MultiLineString', coordinates: lines },
      },
    ],
  }
}

function placesCollection(input) {
  const places = input.features.map((feature) => {
    const p = feature.properties
    const [lng, lat] = feature.geometry.coordinates
    return {
      name: p.name,
      lng: round(lng),
      lat: round(lat),
      minZoom: p.min_zoom,
      capital: p.adm0cap === 1 ? 1 : 0,
      population: p.pop_max,
    }
  })
  places.sort(
    (a, b) =>
      a.minZoom - b.minZoom ||
      b.capital - a.capital ||
      b.population - a.population ||
      a.name.localeCompare(b.name),
  )
  return {
    type: 'FeatureCollection',
    features: places.map((place, rank) => ({
      type: 'Feature',
      properties: { name: place.name, rank },
      geometry: { type: 'Point', coordinates: [place.lng, place.lat] },
    })),
  }
}

const outputs = [
  [INPUTS.countryLines, linesCollection],
  [INPUTS.stateLines, linesCollection],
  [INPUTS.places, placesCollection],
]
for (const [input, build] of outputs) {
  writeFileSync(path.join(outputDir, input.output), JSON.stringify(build(readPinned(input))))
}
