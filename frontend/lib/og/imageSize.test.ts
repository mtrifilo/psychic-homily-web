import { describe, it, expect } from 'vitest'
import { readImageHeader } from './imageSize'

/**
 * Synthetic headers rather than fixture files.
 *
 * The parser only ever reads the first few dozen bytes, so a hand-built header
 * exercises exactly the code under test — and it makes the byte offsets that
 * are the whole substance of this module visible in the test itself.
 */

function png(width: number, height: number): Uint8Array {
  const b = new Uint8Array(24)
  b.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a], 0)
  b.set([0x49, 0x48, 0x44, 0x52], 12) // "IHDR"
  new DataView(b.buffer).setUint32(16, width)
  new DataView(b.buffer).setUint32(20, height)
  return b
}

function gif(width: number, height: number): Uint8Array {
  const b = new Uint8Array(13)
  b.set([...'GIF89a'].map(c => c.charCodeAt(0)), 0)
  new DataView(b.buffer).setUint16(6, width, true)
  new DataView(b.buffer).setUint16(8, height, true)
  return b
}

function webpVp8x(width: number, height: number): Uint8Array {
  const b = new Uint8Array(30)
  b.set([...'RIFF'].map(c => c.charCodeAt(0)), 0)
  b.set([...'WEBP'].map(c => c.charCodeAt(0)), 8)
  b.set([...'VP8X'].map(c => c.charCodeAt(0)), 12)
  const w = width - 1
  const h = height - 1
  b.set([w & 0xff, (w >> 8) & 0xff, (w >> 16) & 0xff], 24)
  b.set([h & 0xff, (h >> 8) & 0xff, (h >> 16) & 0xff], 27)
  return b
}

function webpLossy(width: number, height: number): Uint8Array {
  const b = new Uint8Array(30)
  b.set([...'RIFF'].map(c => c.charCodeAt(0)), 0)
  b.set([...'WEBP'].map(c => c.charCodeAt(0)), 8)
  b.set([...'VP8 '].map(c => c.charCodeAt(0)), 12)
  b.set([0x9d, 0x01, 0x2a], 23)
  new DataView(b.buffer).setUint16(26, width, true)
  new DataView(b.buffer).setUint16(28, height, true)
  return b
}

/**
 * A JPEG with `segmentsBefore` filler segments ahead of the SOF0, so the test
 * covers the segment WALK and not just a fixed offset — the walk is the part
 * that can loop or run off the end.
 */
function jpeg(width: number, height: number, segmentsBefore = 0): Uint8Array {
  const bytes: number[] = [0xff, 0xd8]
  for (let i = 0; i < segmentsBefore; i++) {
    bytes.push(0xff, 0xe0, 0x00, 0x10) // APP0, length 16
    for (let j = 0; j < 14; j++) bytes.push(0x00)
  }
  bytes.push(0xff, 0xc0, 0x00, 0x11, 0x08)
  bytes.push((height >> 8) & 0xff, height & 0xff)
  bytes.push((width >> 8) & 0xff, width & 0xff)
  for (let j = 0; j < 10; j++) bytes.push(0x00)
  return new Uint8Array(bytes)
}

describe('readImageHeader', () => {
  it('reads a PNG', () => {
    expect(readImageHeader(png(1080, 1350))).toEqual({
      width: 1080,
      height: 1350,
      mime: 'image/png',
    })
  })

  it('reads a GIF', () => {
    expect(readImageHeader(gif(500, 750))).toEqual({
      width: 500,
      height: 750,
      mime: 'image/gif',
    })
  })

  // The extended container is what an animated or alpha-channel flyer arrives
  // as, and it stores its size minus one — an off-by-one here is silent.
  it('reads an extended WebP', () => {
    expect(readImageHeader(webpVp8x(1200, 1600))).toEqual({
      width: 1200,
      height: 1600,
      mime: 'image/webp',
    })
  })

  it('reads a lossy WebP', () => {
    expect(readImageHeader(webpLossy(800, 1200))).toEqual({
      width: 800,
      height: 1200,
      mime: 'image/webp',
    })
  })

  it('reads a JPEG whose SOF is the first segment', () => {
    expect(readImageHeader(jpeg(1440, 1800))).toEqual({
      width: 1440,
      height: 1800,
      mime: 'image/jpeg',
    })
  })

  // A real camera JPEG carries EXIF and quantisation tables first, so the SOF
  // is never at a fixed offset.
  it('walks past leading segments to find the SOF', () => {
    expect(readImageHeader(jpeg(1440, 1800, 6))).toEqual({
      width: 1440,
      height: 1800,
      mime: 'image/jpeg',
    })
  })

  it('takes its MIME from the magic bytes, not from any header', () => {
    // The point of the sniff: bytes that ARE a PNG are reported as a PNG no
    // matter what a server claimed in `Content-Type`.
    expect(readImageHeader(png(10, 10))?.mime).toBe('image/png')
  })
})

describe('readImageHeader rejections', () => {
  // This doubles as the content-type guard: whatever an SSRF probe brings back
  // from an internal service is not a parseable image, so it never renders.
  it('rejects a non-image body', () => {
    const html = new TextEncoder().encode('<!doctype html><html><body>hi</body></html>')
    expect(readImageHeader(html)).toBeNull()
    expect(readImageHeader(new TextEncoder().encode('{"secret":"value"}'))).toBeNull()
  })

  it('rejects an empty buffer', () => {
    expect(readImageHeader(new Uint8Array(0))).toBeNull()
  })

  // A truncated file would otherwise be read past its end, producing dimensions
  // for a file we do not have.
  it('rejects a truncated header of a supported format', () => {
    expect(readImageHeader(png(100, 100).subarray(0, 16))).toBeNull()
    expect(readImageHeader(gif(100, 100).subarray(0, 6))).toBeNull()
    expect(readImageHeader(webpVp8x(100, 100).subarray(0, 20))).toBeNull()
  })

  // AVIF and HEIC are deliberately unsupported: the rasteriser cannot decode
  // them, so parsing one would only yield a card with a blank plate.
  it('rejects an AVIF rather than pretending to size it', () => {
    const avif = new Uint8Array(32)
    avif.set([...'\0\0\0 ftypavif'].map(c => c.charCodeAt(0)), 0)
    expect(readImageHeader(avif)).toBeNull()
  })

  // The walk must terminate on garbage rather than spin or read out of bounds.
  it('terminates on a malformed JPEG segment chain', () => {
    const broken = new Uint8Array(200).fill(0xff)
    broken[0] = 0xff
    broken[1] = 0xd8
    expect(readImageHeader(broken)).toBeNull()
  })

  it('terminates on a JPEG whose segment length is nonsense', () => {
    const b = new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x00, ...new Array(40).fill(0)])
    expect(readImageHeader(b)).toBeNull()
  })
})
