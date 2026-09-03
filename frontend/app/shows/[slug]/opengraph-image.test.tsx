import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { deflateSync } from 'node:zlib'

import { isPng, rightmostContentColumn } from '@/lib/og/test-helpers'

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

  // The marked STRING is asserted in `features/shows/showPageDate.test.ts`, and
  // its width budgets in `features/shows/showOgLayout.test.ts`. What only this
  // test can see is that the route still reaches that helper and still returns
  // a card: a zone-less venue must produce different pixels from a zoned one on
  // the same instant and the same fallback zone. Pixel inequality is a weak
  // signal — it cannot say WHICH glyph differed — so read it as a wiring check,
  // not as an assertion that the tilde was drawn.
  it('still renders a card, with a different date row, for a guessed zone', async () => {
    const zoneless = {
      ...BASE_SHOW,
      venues: [{ name: 'Sleeping Village', city: 'Chicago', state: '' }],
    }
    const zoned = {
      ...BASE_SHOW,
      venues: [
        {
          name: 'Sleeping Village',
          city: 'Chicago',
          state: '',
          timezone: 'America/Phoenix',
        },
      ],
    }
    const { res, bytes } = await render(zoneless)
    const { bytes: control } = await render(zoned)

    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
    // Same instant, same fallback zone, same day: the only difference between
    // these two cards is the marker.
    expect(bytes.byteLength).not.toBe(control.byteLength)
  }, 30000)

  // The badge is the one reader-facing CLAIM this card makes about state, and
  // it outlives a correction: a settled card holds for a day with an equal
  // stale-while-revalidate window, so a stale SOLD OUT can survive the page
  // withdrawing it by twice that. Pixel count is the available signal — the
  // badge is a filled rect with text, so it cannot cost zero bytes.
  describe('the SOLD OUT badge', () => {
    const soldOut = { ...BASE_SHOW, is_sold_out: true }

    // `Date` ONLY. Faking the timer functions too would stall the rasteriser,
    // which awaits real async work inside `ImageResponse`.
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ['Date'] })
    })

    it('draws it while the show is still to come', async () => {
      vi.setSystemTime(new Date('2026-09-01T12:00:00Z'))
      const { res, bytes } = await render(soldOut)
      const { bytes: plain } = await render(BASE_SHOW)

      expect(res.status).toBe(200)
      expect(isPng(bytes)).toBe(true)
      expect(bytes.byteLength).toBeGreaterThan(plain.byteLength)
    }, 30000)

    // Withdrawn on the same rule the page uses, so the unfurl cannot keep
    // making a claim the page it links to has retracted.
    it('withholds it once the show is over', async () => {
      vi.setSystemTime(new Date('2026-10-15T12:00:00Z'))
      const { res, bytes } = await render(soldOut)
      const { bytes: plain } = await render(BASE_SHOW)

      expect(res.status).toBe(200)
      expect(isPng(bytes)).toBe(true)
      expect(bytes.byteLength).toBe(plain.byteLength)
    }, 30000)

    // A venue-less show is judged on the SHOW's own state. The instant and
    // clock below are chosen so the two candidate zones DISAGREE, which is
    // what makes this discriminating: at 12:00Z on Oct 1, an event at
    // 05:00Z is still `today` in Chicago (00:00 local, same day) but already
    // `past` in Phoenix (22:00 local the day before). So the badge appears
    // only if the fallback reaches `state: 'IL'`; on the default zone the
    // show reads as archived and the badge is withheld.
    it('judges a venue-less show on its own state, not the default zone', async () => {
      const atMidnightChicago = {
        venues: [],
        state: 'IL',
        event_date: '2026-10-01T05:00:00Z',
      }
      vi.setSystemTime(new Date('2026-10-01T12:00:00Z'))

      const { bytes } = await render({ ...soldOut, ...atMidnightChicago })
      const { bytes: plain } = await render({ ...BASE_SHOW, ...atMidnightChicago })

      expect(bytes.byteLength).toBeGreaterThan(plain.byteLength)
    }, 30000)

    afterEach(() => {
      vi.useRealTimers()
    })
  })

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
