/**
 * Pixel assertions for the OG share-card routes.
 *
 * Layout regressions on these cards are SILENT — nothing throws when a column
 * narrows or an element stops being drawn, the picture is just slightly wrong —
 * so the only honest assertion is against the pixels. That means decoding the
 * PNG the route actually emits, and every route test that does it was
 * hand-rolling the same chunk walk and the same Paeth unfilter. Two copies of a
 * decoder with no tests of its own means a defect in one is a false green in
 * the other.
 *
 * TEST-ONLY, and it must stay that way: nothing here may be imported by a route.
 * These functions are cheap, but the OG routes are edge functions at ~96% of
 * Vercel's 1 MB limit, and an accidental import would be paid for at deploy.
 * The filename is the guard — `*.test.{ts,tsx}` is what vitest collects, so this
 * is a plain module that only tests reference.
 *
 * Decoding is cheap because the routes emit 8-bit RGBA with no interlacing.
 */

import { inflateSync } from 'node:zlib'

/** PNG magic — proof a real raster came back rather than an error page. */
export function isPng(bytes: Uint8Array): boolean {
  return bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47
}

export interface DecodedPng {
  width: number
  height: number
  /** RGBA, row-major, filters already undone. */
  pixels: Uint8Array
}

export function decodeRgba(png: Uint8Array): DecodedPng {
  const view = new DataView(png.buffer, png.byteOffset, png.byteLength)
  let at = 8
  let width = 0
  let height = 0
  const idat: Buffer[] = []
  while (at < png.byteLength) {
    const length = view.getUint32(at)
    const type = String.fromCharCode(png[at + 4], png[at + 5], png[at + 6], png[at + 7])
    const data = png.subarray(at + 8, at + 8 + length)
    if (type === 'IHDR') {
      width = view.getUint32(at + 8)
      height = view.getUint32(at + 12)
    } else if (type === 'IDAT') {
      idat.push(Buffer.from(data))
    } else if (type === 'IEND') break
    at += 12 + length
  }

  const raw = inflateSync(Buffer.concat(idat))
  const stride = width * 4
  const pixels = new Uint8Array(width * height * 4)
  // Undo the per-row filters. Paeth included: resvg emits it on real output.
  for (let y = 0; y < height; y += 1) {
    const filter = raw[y * (stride + 1)]
    const row = y * (stride + 1) + 1
    for (let i = 0; i < stride; i += 1) {
      const value = raw[row + i]
      const left = i >= 4 ? pixels[y * stride + i - 4] : 0
      const up = y > 0 ? pixels[(y - 1) * stride + i] : 0
      const upLeft = y > 0 && i >= 4 ? pixels[(y - 1) * stride + i - 4] : 0
      let recovered: number
      switch (filter) {
        case 1:
          recovered = value + left
          break
        case 2:
          recovered = value + up
          break
        case 3:
          recovered = value + ((left + up) >> 1)
          break
        case 4: {
          const p = left + up - upLeft
          const pa = Math.abs(p - left)
          const pb = Math.abs(p - up)
          const pc = Math.abs(p - upLeft)
          recovered = value + (pa <= pb && pa <= pc ? left : pb <= pc ? up : upLeft)
          break
        }
        default:
          recovered = value
      }
      pixels[y * stride + i] = recovered & 0xff
    }
  }
  return { width, height, pixels }
}

/** An `#rrggbb` string as the `[r, g, b]` the pixel buffer holds. */
export function rgb(hex: string): [number, number, number] {
  const value = Number.parseInt(hex.replace('#', ''), 16)
  return [(value >> 16) & 0xff, (value >> 8) & 0xff, value & 0xff]
}

interface ColumnBand {
  /** Inclusive left edge; clamped to the image. */
  fromX?: number
  /** Exclusive right edge; clamped to the image. */
  toX?: number
}

/**
 * How many pixels in a column band are within `tolerance` of `colour`.
 *
 * A tolerance rather than an exact match, because cards draw with fill-opacity
 * over the brand background and resvg antialiases every edge.
 */
export function countPixelsNear(
  png: Uint8Array,
  colour: readonly [number, number, number],
  tolerance: number,
  band: ColumnBand = {}
): number {
  const { width, height, pixels } = decodeRgba(png)
  const from = Math.max(0, band.fromX ?? 0)
  const to = Math.min(width, band.toX ?? width)
  let found = 0
  for (let y = 0; y < height; y += 1) {
    for (let x = from; x < to; x += 1) {
      const i = (y * width + x) * 4
      if (
        Math.abs(pixels[i] - colour[0]) < tolerance &&
        Math.abs(pixels[i + 1] - colour[1]) < tolerance &&
        Math.abs(pixels[i + 2] - colour[2]) < tolerance
      ) {
        found += 1
      }
    }
  }
  return found
}

/**
 * How many pixels in a column band differ from the card's background.
 *
 * The background is sampled from the top-left pixel, which every card in the
 * family fills, so this needs no colour argument and stays correct if the brand
 * token moves.
 */
export function countNonBackgroundPixels(png: Uint8Array, band: ColumnBand = {}): number {
  const { width, height, pixels } = decodeRgba(png)
  const bg = [pixels[0], pixels[1], pixels[2]] as const
  const from = Math.max(0, band.fromX ?? 0)
  const to = Math.min(width, band.toX ?? width)
  let found = 0
  for (let y = 0; y < height; y += 1) {
    for (let x = from; x < to; x += 1) {
      const i = (y * width + x) * 4
      if (
        Math.abs(pixels[i] - bg[0]) > 6 ||
        Math.abs(pixels[i + 1] - bg[1]) > 6 ||
        Math.abs(pixels[i + 2] - bg[2]) > 6
      ) {
        found += 1
      }
    }
  }
  return found
}

/**
 * The x of the right-most pixel that is not the card background, or -1.
 *
 * What a fixed-width text column is asserted against: the budgets say where the
 * content should stop, and this is where it actually stopped.
 */
export function rightmostContentColumn(png: Uint8Array): number {
  const { width, height, pixels } = decodeRgba(png)
  const bg = [pixels[0], pixels[1], pixels[2]] as const
  for (let x = width - 1; x >= 0; x -= 1) {
    for (let y = 0; y < height; y += 1) {
      const i = (y * width + x) * 4
      if (
        Math.abs(pixels[i] - bg[0]) > 6 ||
        Math.abs(pixels[i + 1] - bg[1]) > 6 ||
        Math.abs(pixels[i + 2] - bg[2]) > 6
      ) {
        return x
      }
    }
  }
  return -1
}
