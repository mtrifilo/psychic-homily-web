// Shared by the scripts that build the compact Atlas globe's same-origin
// files from Natural Earth (public domain, no credit required:
// https://www.naturalearthdata.com/about/terms-of-use/).
//
// Every input is pinned to one natural-earth-vector commit:
// https://raw.githubusercontent.com/nvkelso/natural-earth-vector/693f11422f4e08d2da4566b854dda53eb7c39fb3/geojson/<name>.geojson
// and checked against its sha256 before anything is built from it.
//
// Coordinates are rounded to COORD_DECIMALS: 0.01 degree is about 1 km, under
// a pixel below z5.5 (where the globe surface is fully opaque) and under two
// at z7 (where it has faded out) at the equator; the globe magnifies by
// 1/cos(latitude), so about 2.5 px at Chicago at z7.
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'

export const COORD_DECIMALS = 2

const factor = 10 ** COORD_DECIMALS
export const round = (n) => Math.round(n * factor) / factor

/** The positions rounded, with consecutive duplicates removed. */
export function compactPositions(positions) {
  const out = []
  for (const [lng, lat] of positions) {
    const p = [round(lng), round(lat)]
    const last = out[out.length - 1]
    if (!last || last[0] !== p[0] || last[1] !== p[1]) out.push(p)
  }
  return out
}

/** The parsed input at `path`, after checking it is the pinned file. */
export function readPinned(path, sha256) {
  const raw = readFileSync(path)
  const actual = createHash('sha256').update(raw).digest('hex')
  if (actual !== sha256) {
    console.error(`${path}: sha256 ${actual} does not match the pinned ${sha256}`)
    process.exit(1)
  }
  return JSON.parse(raw.toString('utf8'))
}
