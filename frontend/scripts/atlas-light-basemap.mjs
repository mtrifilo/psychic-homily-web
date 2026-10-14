#!/usr/bin/env node
// Writes features/scenes/basemap/ph-light-basemap.json from
// ph-dark-basemap.json and the light palette in lib/ph-light-basemap.mjs.
// Run it after any change to either; phBasemap.test.ts fails while the
// committed light file differs from what this writes.
//
// Usage (from frontend/):
//   node scripts/atlas-light-basemap.mjs
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { lightBasemapText } from './lib/ph-light-basemap.mjs'

const basemapDir = join(dirname(fileURLToPath(import.meta.url)), '../features/scenes/basemap')
const darkText = readFileSync(join(basemapDir, 'ph-dark-basemap.json'), 'utf8')
const outPath = join(basemapDir, 'ph-light-basemap.json')
writeFileSync(outPath, lightBasemapText(darkText))
console.log(`wrote ${outPath}`)
