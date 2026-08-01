import { describe, it, expect, vi, afterEach } from 'vitest'
import { deflateSync, inflateSync } from 'node:zlib'

/**
 * The route actually rendering, not just the pure modules it composes.
 *
 * Worth the cost because the riskiest line in this feature has no other cover:
 * `<img src={plate.src as unknown as string}>` hands Satori an `ArrayBuffer`
 * through a prop React types as `string`. Nothing about that survives type
 * checking, and per this repo's own experience OG failures surface at DEPLOY
 * rather than at build — so a `next/og` bump that changes the object branch of
 * its image resolver would otherwise ship green and break in production.
 *
 * The other thing these assert is the contract the whole design rests on: the
 * route does not 5xx. Every case below feeds it something hostile and expects
 * a 200 with a real PNG.
 */

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

const BASE_SHOW = {
  title: 'Militarie Gun at Sleeping Village',
  event_date: '2026-09-30T02:00:00Z',
  is_sold_out: false,
  is_cancelled: false,
  image_url: null as string | null,
  venues: [{ name: 'Sleeping Village', city: 'Chicago', state: 'IL', timezone: 'America/Chicago' }],
  artists: [{ name: 'Militarie Gun', is_headliner: true }, { name: 'MSPAINT' }],
}

/**
 * A real, decodable PNG — header-only bytes would size correctly and then
 * rasterise to nothing, so the plate assertion would pass without the image
 * ever having been drawn.
 */
function png(width: number, height: number): Uint8Array {
  const raw = Buffer.alloc(height * (1 + width * 3))
  for (let y = 0; y < height; y++) {
    const row = y * (1 + width * 3)
    raw[row] = 0 // filter: none
    for (let x = 0; x < width; x++) {
      raw[row + 1 + x * 3] = 220 // a solid, high-contrast plate
      raw[row + 2 + x * 3] = 40
      raw[row + 3 + x * 3] = 90
    }
  }

  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(width, 0)
  ihdr.writeUInt32BE(height, 4)
  ihdr[8] = 8 // bit depth
  ihdr[9] = 2 // colour type: truecolour

  return new Uint8Array(
    Buffer.concat([
      Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
      chunk('IHDR', ihdr),
      chunk('IDAT', deflateSync(raw)),
      chunk('IEND', Buffer.alloc(0)),
    ])
  )
}

function chunk(type: string, data: Buffer): Buffer {
  const length = Buffer.alloc(4)
  length.writeUInt32BE(data.length, 0)
  const body = Buffer.concat([Buffer.from(type, 'ascii'), data])
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(body), 0)
  return Buffer.concat([length, body, crc])
}

function crc32(buf: Buffer): number {
  let c = ~0
  for (let i = 0; i < buf.length; i++) {
    c ^= buf[i]
    for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1))
  }
  return ~c >>> 0
}

/** Serves the show JSON on the API host and `flyer` on anything else. */
function stubBackend(show: object, flyer?: Uint8Array | { status: number }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request) => {
      const url = String(input)
      if (url.includes('/shows/')) {
        return new Response(JSON.stringify(show), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      }
      if (!flyer) return new Response('nope', { status: 404 })
      if (flyer instanceof Uint8Array) {
        const buf = flyer.buffer.slice(
          flyer.byteOffset,
          flyer.byteOffset + flyer.byteLength
        ) as ArrayBuffer
        return new Response(buf, {
          status: 200,
          headers: { 'content-length': String(flyer.byteLength) },
        })
      }
      return new Response('nope', { status: flyer.status })
    })
  )
}

async function render(show: object, flyer?: Uint8Array | { status: number }) {
  stubBackend(show, flyer)
  const mod = await import('./opengraph-image')
  const res = await mod.default({ params: Promise.resolve({ slug: 'test-show' }) })
  const bytes = new Uint8Array(await res.arrayBuffer())
  return { res, bytes }
}

/** PNG magic — proof a real raster came back rather than an error page. */
function isPng(bytes: Uint8Array): boolean {
  return bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47
}

/**
 * The x of the right-most pixel that is not the card background.
 *
 * Layout regressions on this card are silent — nothing throws when a column
 * narrows, the text just sits somewhere slightly different — so the only honest
 * assertion is against the pixels. Decoding is cheap here because the route
 * emits 8-bit RGBA with no interlacing.
 */
function rightmostContentColumn(png: Uint8Array): number {
  const { width, height, pixels } = decodeRgba(png)
  const bg = [pixels[0], pixels[1], pixels[2]]
  for (let x = width - 1; x >= 0; x--) {
    for (let y = 0; y < height; y++) {
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

function decodeRgba(png: Uint8Array) {
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
  for (let y = 0; y < height; y++) {
    const filter = raw[y * (stride + 1)]
    const src = y * (stride + 1) + 1
    const dst = y * stride
    for (let i = 0; i < stride; i++) {
      const a = i >= 4 ? pixels[dst + i - 4] : 0
      const b = y > 0 ? pixels[dst - stride + i] : 0
      const c = i >= 4 && y > 0 ? pixels[dst - stride + i - 4] : 0
      let value = raw[src + i]
      if (filter === 1) value += a
      else if (filter === 2) value += b
      else if (filter === 3) value += (a + b) >> 1
      else if (filter === 4) {
        const p = a + b - c
        const pa = Math.abs(p - a)
        const pb = Math.abs(p - b)
        const pc = Math.abs(p - c)
        value += pa <= pb && pa <= pc ? a : pb <= pc ? b : c
      }
      pixels[dst + i] = value & 0xff
    }
  }
  return { width, height, pixels }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('the show OG route', () => {
  it('renders a card with the flyer plate', async () => {
    const withFlyer = { ...BASE_SHOW, image_url: 'https://cdn.example.com/flyer.png' }
    const { res, bytes } = await render(withFlyer, png(1000, 1500))
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)

    // The plate has to actually contribute pixels, which is what proves the
    // ArrayBuffer `src` reached the rasteriser rather than being ignored.
    const { bytes: textOnly } = await render(BASE_SHOW)
    expect(bytes.byteLength).toBeGreaterThan(textOnly.byteLength)
  }, 30000)

  it('renders the text-only card for a flyer-less show', async () => {
    const { res, bytes } = await render(BASE_SHOW)
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
  }, 30000)

  // The cancelled flyer-less card is where an unconditional row `gap` bit: with
  // the absolute overlay as a sibling, Yoga reserved the gap anyway and the
  // column silently lost 40px it had already been measured against. Rendering
  // both and comparing the right-hand content edge is what catches that.
  it('gives the cancelled flyer-less card the same column as the plain one', async () => {
    const { bytes: plain } = await render(BASE_SHOW)
    const { bytes: cancelled } = await render({ ...BASE_SHOW, is_cancelled: true })
    expect(rightmostContentColumn(cancelled)).toBe(rightmostContentColumn(plain))
  }, 30000)

  // The contract the whole fail-open design exists for.
  it.each([
    ['an unreachable flyer host', { status: 500 } as const],
    ['a 404 flyer', { status: 404 } as const],
  ])('returns 200 for %s', async (_label, flyer) => {
    const show = { ...BASE_SHOW, image_url: 'https://cdn.example.com/flyer.png' }
    const { res, bytes } = await render(show, flyer)
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
  }, 30000)

  // Formats the rasteriser cannot draw must be dropped BEFORE it sees them —
  // handed one, it throws from inside the response stream.
  it('returns 200 for a WebP flyer', async () => {
    const webp = new Uint8Array(30)
    webp.set([...'RIFF'].map(c => c.charCodeAt(0)), 0)
    webp.set([...'WEBP'].map(c => c.charCodeAt(0)), 8)
    webp.set([...'VP8X'].map(c => c.charCodeAt(0)), 12)
    const show = { ...BASE_SHOW, image_url: 'https://cdn.example.com/flyer.webp' }
    const { res, bytes } = await render(show, webp)
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
  }, 30000)

  // A 21-byte frame header with no scan data behind it: sized cleanly under an
  // earlier parser and then trapped the rasteriser mid-stream.
  it('returns 200 for a JPEG that would trap the rasteriser', async () => {
    const trap = new Uint8Array([
      0xff, 0xd8, 0xff, 0xc0, 0x00, 0x11, 0x08, 0x01, 0xf4, 0x01, 0x90, 0x00, 0x00, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    ])
    const show = { ...BASE_SHOW, image_url: 'https://cdn.example.com/trap.jpg' }
    const { res, bytes } = await render(show, trap)
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
  }, 30000)

  it('returns 200 when the show itself cannot be fetched', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('nope', { status: 500 })))
    const mod = await import('./opengraph-image')
    const res = await mod.default({ params: Promise.resolve({ slug: 'missing' }) })
    expect(res.status).toBe(200)
    expect(isPng(new Uint8Array(await res.arrayBuffer()))).toBe(true)
  }, 30000)
})

/**
 * Asserted RELATIVE to a flyer-less render rather than against literal seconds:
 * the brand fonts are route assets that do not resolve under vitest, so every
 * card here is `degraded` and already on the short window. The invariant these
 * protect is the classification, not the number.
 */
describe('how long a card may be cached', () => {
  const flyerShow = { ...BASE_SHOW, image_url: 'https://cdn.example.com/flyer.png' }
  const ttl = (res: Response) => res.headers.get('cache-control')

  // A 5xx could succeed on the next unfurl, so the card is deliberately held
  // briefly rather than baking a blip in for the full window.
  it('puts a transiently-failed flyer on the short window', async () => {
    const { res } = await render(flyerShow, { status: 503 })
    expect(ttl(res)).toContain('s-maxage=60')
  }, 30000)

  // A rejected flyer is a PERMANENT fact about the row, so it must cache
  // exactly like an ordinary card — re-rendering it every 60s forever is the
  // cost `ogCacheControl` exists to avoid.
  it('caches a rejected flyer exactly like a flyer-less show', async () => {
    const httpFlyer = { ...BASE_SHOW, image_url: 'http://cdn.example.com/flyer.png' }
    const { res: rejected } = await render(httpFlyer)
    const { res: plain } = await render(BASE_SHOW)
    expect(ttl(rejected)).toBe(ttl(plain))
  }, 30000)

  it('caches a rendered flyer exactly like a flyer-less show', async () => {
    const { res: withPlate } = await render(flyerShow, png(200, 300))
    const { res: plain } = await render(BASE_SHOW)
    expect(ttl(withPlate)).toBe(ttl(plain))
  }, 30000)
})
