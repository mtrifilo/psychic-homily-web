import { describe, it, expect, vi, afterEach } from 'vitest'

/**
 * The route actually rendering, not just the pure modules it composes.
 *
 * Worth the cost for the same reason the show card's route test is: the riskiest
 * thing on this card has no other cover. The map motif is an INLINE `<svg>`,
 * which Satori serialises to a data URI and hands to resvg as an image — a path
 * nothing about type checking exercises, and one a `next/og` bump could change
 * silently. Per this repo's own experience OG failures surface at DEPLOY rather
 * than at build, so "it compiled" is not evidence that a dot was drawn.
 *
 * The other thing these assert is the contract the design rests on: the route
 * does not 5xx. Every case below feeds it something hostile — no snapshot, an
 * error page served as 200, a snapshot with no arrival dates — and expects a 200
 * with a real PNG.
 */

import { OG_COLORS } from '@/lib/og/brand'
import {
  countNonBackgroundPixels,
  countPixelsNear,
  decodeRgba,
  isPng,
  rgb,
} from '@/lib/og/test-helpers'

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

const QUANT = 32767

function encodeBytes(bytes: number[]): string {
  return Buffer.from(bytes).toString('base64')
}

const EPOCH = '2020-01-01T00:00:00Z'
const LAST_MAPPED = '2026-08-02T04:30:00Z'

function appearAt(iso: string): number {
  return Math.floor((new Date(iso).getTime() - new Date(EPOCH).getTime()) / 1000)
}

/** In the JUL 27 - AUG 2 window; long before it. */
const NEW = appearAt('2026-07-30T00:00:00Z')
const OLD = appearAt('2021-03-01T00:00:00Z')

/**
 * Eight artists, positioned deliberately rather than decoratively.
 *
 * `MOTIF_BOX` OVERHANGS the canvas by design — that bleed is the composition —
 * so the two corner nodes here exist only to pin the projection's bounds and
 * both land off-canvas. What the assertions need is arrivals that land INSIDE
 * the right-hand strip (x ≥ 1120), where no text is ever drawn at any size, so
 * an orange pixel out there can only have come from the SVG. Nodes 3, 7 and 8
 * are those; a first draft of this fixture used only the four corners and every
 * dot fell off the canvas, which is exactly the failure this comment exists to
 * stop someone re-introducing.
 */
function overviewFixture(overrides: Record<string, unknown> = {}) {
  return {
    version: 1,
    last_mapped: LAST_MAPPED,
    epoch: EPOCH,
    extent: 500,
    node_count: 8,
    edge_count: 3,
    isolate_count: 4,
    rank_metric: 'betweenness',
    hull_kind: 'convex',
    nodes: {
      id: [1, 2, 3, 4, 5, 6, 7, 8],
      kind: encodeBytes([0, 0, 0, 0, 0, 0, 0, 0]),
      name: ['Alpha', 'Beta', 'Gamma', 'Delta', 'Epsilon', 'Zeta', 'Eta', 'Theta'],
      slug: ['alpha', 'beta', 'gamma', 'delta', 'epsilon', 'zeta', 'eta', 'theta'],
      //     bounds        bounds  right    right-ish  centre  mid-left  right   right
      x: [-QUANT, QUANT, 24903, 20000, 0, -10000, 26000, 22000],
      y: [-QUANT, QUANT, 0, 5000, 0, 10000, -3000, 8000],
      community: [0, 0, 1, 1, 0, 1, 1, 1],
      degree: [1, 1, 1, 1, 1, 0, 1, 1],
      rank: [0, 1, 2, 3, 4, 5, 6, 7],
      flags: encodeBytes([0, 0, 0, 0, 0, 0, 0, 0]),
      appear: [OLD, OLD, NEW, NEW, OLD, OLD, NEW, NEW],
    },
    edges: {
      // CSR, both directions: 1—5 (old), 3—7 (new), 4—8 (new).
      offsets: [0, 1, 1, 2, 3, 4, 4, 5, 6],
      targets: [4, 6, 7, 0, 2, 3],
      kind: encodeBytes([0, 0, 0, 0, 0, 0]),
      appear: [OLD, NEW, NEW, OLD, NEW, NEW],
    },
    regions: [],
    ...overrides,
  }
}

/** Serves `body` on the overview endpoint, and the font assets from disk. */
function stubBackend(body: unknown, status = 200) {
  const realFetch = globalThis.fetch
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/graph/overview')) {
        if (status !== 200) return new Response('nope', { status })
        return new Response(typeof body === 'string' ? body : JSON.stringify(body), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      }
      // The brand fonts are route assets read through `fetch(new URL(...))`.
      return realFetch(input as RequestInfo, init)
    })
  )
}

async function render(body: unknown, status = 200) {
  stubBackend(body, status)
  const mod = await import('./opengraph-image')
  const res = await mod.default()
  const bytes = new Uint8Array(await res.arrayBuffer())
  return { res, bytes }
}

/** The brand primary, `#e89960`, as the card paints its arrivals and counts. */
const PRIMARY = rgb(OG_COLORS.primary)

/** Tolerance: the dots carry a fill-opacity and resvg antialiases their edges. */
const PRIMARY_TOLERANCE = 34

afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetModules()
})

describe('graph this-week OG card', () => {
  it('renders a 1200×630 PNG from a snapshot', async () => {
    const { res, bytes } = await render(overviewFixture())

    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
    const { width, height } = decodeRgba(bytes)
    expect(width).toBe(1200)
    expect(height).toBe(630)
  })

  it('actually rasterises the inline SVG motif', async () => {
    // THE point of this file. The right-hand strip carries no text at any size,
    // so an orange pixel out there can only be a highlighted arrival — and if
    // Satori ever stops rendering the inline `<svg>`, this is the only test that
    // notices before a deploy does.
    const { bytes } = await render(overviewFixture())

    expect(countPixelsNear(bytes, PRIMARY, PRIMARY_TOLERANCE, { fromX: 1120 })).toBeGreaterThan(0)
    expect(countNonBackgroundPixels(bytes, { fromX: 1120 })).toBeGreaterThan(0)
  })

  it('paints the counts line in the brand primary on the left', async () => {
    const { bytes } = await render(overviewFixture())
    expect(countPixelsNear(bytes, PRIMARY, PRIMARY_TOLERANCE, { fromX: 72, toX: 400 })).toBeGreaterThan(0)
  })

  // Every case below is the FALLBACK branch: the route must still answer with a
  // card. An unfurler handed a 500 shows nothing at all, and some clients then
  // cache the miss.
  it('falls back to the branded card when no snapshot has been built', async () => {
    const { res, bytes } = await render(null, 503)
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
    // No motif: nothing on the right-hand strip at all.
    expect(countNonBackgroundPixels(bytes, { fromX: 1120 })).toBe(0)
  })

  it('falls back when a 200 carries something that is not a snapshot', async () => {
    const { res, bytes } = await render({ error: 'not found' })
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
  })

  it('falls back when the body is not JSON at all', async () => {
    const { res, bytes } = await render('<html>gateway timeout</html>')
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
  })

  it('falls back when the snapshot carries no arrival dates', async () => {
    // Every node reading 0 is how a payload with no `appear` column decodes.
    // There is no week in it, so there is no card to draw from it.
    const undated = overviewFixture()
    undated.nodes.appear = undated.nodes.appear.map(() => 0)
    undated.edges.appear = undated.edges.appear.map(() => 0)

    const { res, bytes } = await render(undated)
    expect(res.status).toBe(200)
    expect(isPng(bytes)).toBe(true)
    expect(countNonBackgroundPixels(bytes, { fromX: 1120 })).toBe(0)
  })

  it('sets a CDN cache window at all — next/og would otherwise forbid one', async () => {
    // Held to the SHORT window here, and that is correct rather than a bug: the
    // brand `.ttf` files are route assets that vitest cannot serve, so every card
    // rendered under this suite is `degraded` and takes the fallback window by
    // design. What this asserts is the part the environment cannot fake —
    // `next/og` overwrites its own long-lived default with
    // `max-age=0, must-revalidate` unless a caller passes a header, so without
    // `ogCacheControl` every single unfurl would re-run the edge function.
    const { res } = await render(overviewFixture())
    expect(res.headers.get('cache-control')).toBe(
      'public, max-age=0, s-maxage=60, stale-while-revalidate=60'
    )
  })
})
