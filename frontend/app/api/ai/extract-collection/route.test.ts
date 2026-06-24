import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
  afterEach,
  type Mock,
} from 'vitest'

// --- Anthropic SDK mock ---------------------------------------------------
//
// Mirrors extract-show/route.test.ts: the route does
//   - `import Anthropic from '@anthropic-ai/sdk'`
//   - `new Anthropic({ apiKey })`
//   - `anthropic.messages.create(...)`
//   - `error instanceof Anthropic.APIError`
// so the mocked default export is a class whose instances expose a
// `messages.create` we control, carrying a real `APIError` static so the
// route's `instanceof` branch stays reachable. (These PSY-856 tests don't
// exercise the APIError branch — it's covered by extract-show/route.test.ts —
// but the static must exist for the route's `instanceof` checks at import.)
const { mockCreate, MockAnthropic } = vi.hoisted(() => {
  const create: Mock = vi.fn()

  class HoistedAPIError extends Error {
    status: number | undefined
    headers: unknown
    error: unknown
    constructor(
      status: number | undefined,
      error: unknown,
      message: string | undefined,
      headers: unknown
    ) {
      super(message)
      this.name = 'APIError'
      this.status = status
      this.error = error
      this.headers = headers
    }
  }

  class HoistedAnthropic {
    messages: { create: Mock }
    static APIError = HoistedAPIError
    constructor(_opts: { apiKey: string }) {
      this.messages = { create }
    }
  }

  return {
    mockCreate: create,
    MockAnthropic: HoistedAnthropic,
  }
})

vi.mock('@anthropic-ai/sdk', () => ({
  default: MockAnthropic,
}))

// --- Sentry mock ----------------------------------------------------------
const { mockCaptureException } = vi.hoisted(() => ({
  mockCaptureException: vi.fn(),
}))
vi.mock('@sentry/nextjs', () => ({
  captureException: mockCaptureException,
  captureMessage: vi.fn(),
}))

// --- next/headers cookies mock --------------------------------------------
let mockAuthCookie: { value: string } | undefined = { value: 'valid-token' }
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({
    get: (name: string) =>
      name === 'auth_token' ? mockAuthCookie : undefined,
  })),
}))

// Import AFTER mocks are registered.
import { POST, maxDuration } from './route'
import type {
  ExtractCollectionRequest,
  ExtractCollectionResponse,
} from '@/lib/types/extraction'

// --- fetch router ---------------------------------------------------------
//
// The route calls fetch for three backend endpoints:
//   /auth/profile         — auth verification
//   /ai-extraction/throttle — PSY-855 rate-limit gate
//   /artists/search       — per-row artist match
//
// `artistResults` maps the search query (the artist name) to the response
// body. `searchHook` lets a test observe each in-flight search (used by the
// bounded-concurrency test) and optionally control timing/failure.
interface ArtistSearchBody {
  artists: Array<{ id: number; name: string; slug: string }>
  count: number
}

interface FetchConfig {
  profileOk: boolean
  profileBody: unknown
  throttleOk: boolean
  throttleBody: unknown
  artistResults: Record<string, ArtistSearchBody>
  // Optional async hook invoked with the query for every /artists/search call,
  // BEFORE the response resolves. Lets a test gate resolution + count in-flight.
  searchHook?: (q: string) => Promise<void>
  // Optional: queries listed here make /artists/search return !ok (a failed
  // backend search). The route's searchArtist swallows this to null.
  searchFailFor?: Set<string>
}

let fetchConfig: FetchConfig

function makeResponse(ok: boolean, body: unknown): Response {
  return {
    ok,
    status: ok ? 200 : 400,
    json: async () => body,
  } as unknown as Response
}

function installFetch() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()

    if (url.includes('/ai-extraction/throttle')) {
      return makeResponse(fetchConfig.throttleOk, fetchConfig.throttleBody)
    }
    if (url.includes('/auth/profile')) {
      return makeResponse(fetchConfig.profileOk, fetchConfig.profileBody)
    }
    if (url.includes('/artists/search')) {
      const q = new URL(url).searchParams.get('q') ?? ''
      if (fetchConfig.searchHook) {
        await fetchConfig.searchHook(q)
      }
      if (fetchConfig.searchFailFor?.has(q)) {
        return makeResponse(false, { error: 'search exploded' })
      }
      const body = fetchConfig.artistResults[q] ?? { artists: [], count: 0 }
      return makeResponse(true, body)
    }
    throw new Error(`Unexpected fetch URL in test: ${url}`)
  })
  vi.stubGlobal('fetch', fetchMock)
}

function claudeTextResponse(payload: unknown) {
  return {
    content: [{ type: 'text', text: JSON.stringify(payload) }],
  }
}

function makeRequest(body: ExtractCollectionRequest | string) {
  return {
    json: async () => {
      if (typeof body === 'string') throw new Error('invalid json')
      return body
    },
  } as unknown as Parameters<typeof POST>[0]
}

const ORIGINAL_API_KEY = process.env.ANTHROPIC_API_KEY

beforeEach(() => {
  vi.clearAllMocks()
  process.env.ANTHROPIC_API_KEY = 'test-key'
  mockAuthCookie = { value: 'valid-token' }
  fetchConfig = {
    profileOk: true,
    profileBody: { success: true, user: { id: 'u1' } },
    throttleOk: true,
    throttleBody: {
      allowed: true,
      retry_after_seconds: 0,
      limit: 10,
      window_seconds: 3600,
    },
    artistResults: {},
  }
  installFetch()
})

afterEach(() => {
  vi.unstubAllGlobals()
  if (ORIGINAL_API_KEY === undefined) {
    delete process.env.ANTHROPIC_API_KEY
  } else {
    process.env.ANTHROPIC_API_KEY = ORIGINAL_API_KEY
  }
})

async function readJson(res: Response): Promise<ExtractCollectionResponse> {
  return (await res.json()) as ExtractCollectionResponse
}

describe('POST /api/ai/extract-collection', () => {
  it('declares maxDuration = 60 (PSY-856 defense-in-depth)', () => {
    expect(maxDuration).toBe(60)
  })

  describe('per-row matching (sanity)', () => {
    it('auto-matches a single exact artist and carries release_title through', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          source: 'Best of 2015',
          items: [{ artist_name: 'Sleep', release_title: 'The Sciences' }],
        })
      )
      fetchConfig.artistResults['Sleep'] = {
        artists: [{ id: 42, name: 'Sleep', slug: 'sleep' }],
        count: 1,
      }

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      const body = await readJson(res)

      expect(res.status).toBe(200)
      const item = body.data?.items[0]
      expect(item?.matched_artist_id).toBe(42)
      expect(item?.matched_artist_name).toBe('Sleep')
      expect(item?.matched_artist_slug).toBe('sleep')
      expect(item?.release_title).toBe('The Sciences')
    })

    it('surfaces suggestions (not an auto-match) when multiple names exact-match', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({ items: [{ artist_name: 'Boris' }] })
      )
      // Two distinct PH artists both literally named "Boris" — must NOT
      // auto-pick (same-name-band collision guard).
      fetchConfig.artistResults['Boris'] = {
        artists: [
          { id: 1, name: 'Boris', slug: 'boris-jp' },
          { id: 2, name: 'Boris', slug: 'boris-us' },
        ],
        count: 2,
      }

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      const body = await readJson(res)

      const item = body.data?.items[0]
      expect(item?.matched_artist_id).toBeUndefined()
      expect(item?.artist_suggestions).toHaveLength(2)
    })

    it('skips rows with no usable artist name', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          items: [
            { artist_name: 'Real Band' },
            { artist_name: '   ' }, // whitespace-only → skipped
            { release_title: 'Orphan release with no artist' }, // no name → skipped
          ],
        })
      )

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      const body = await readJson(res)

      expect(body.data?.items).toHaveLength(1)
      expect(body.data?.items[0].artist_name).toBe('Real Band')
    })
  })

  // --- PSY-856: bounded-parallel match pass -------------------------------
  describe('bounded-parallel matching (PSY-856)', () => {
    it('preserves source order across a 60-item batch regardless of completion order', async () => {
      // 60 rows: artist-0 .. artist-59, each matching a distinct DB id (i+1000).
      // To prove order is independent of *completion* order, make the search
      // resolution time the INVERSE of position — later rows resolve first.
      const N = 60
      const items = Array.from({ length: N }, (_, i) => ({
        artist_name: `artist-${i}`,
        release_title: `release-${i}`,
      }))
      for (let i = 0; i < N; i++) {
        fetchConfig.artistResults[`artist-${i}`] = {
          artists: [{ id: 1000 + i, name: `artist-${i}`, slug: `artist-${i}` }],
          count: 1,
        }
      }
      fetchConfig.searchHook = async (q: string) => {
        const i = Number(q.split('-')[1])
        // Earlier index → longer delay → finishes later than later indices.
        await new Promise(r => setTimeout(r, (N - i) % 7))
      }
      mockCreate.mockResolvedValue(claudeTextResponse({ items }))

      const res = await POST(makeRequest({ type: 'text', text: 'big list' }))
      const body = await readJson(res)

      expect(res.status).toBe(200)
      expect(body.data?.items).toHaveLength(N)
      // Result array is index-aligned with the input order, not resolution order.
      body.data?.items.forEach((item, i) => {
        expect(item.artist_name).toBe(`artist-${i}`)
        expect(item.matched_artist_id).toBe(1000 + i)
        expect(item.release_title).toBe(`release-${i}`)
      })
    })

    it('never exceeds 20 concurrent /artists/search calls (the p-limit cap)', async () => {
      const N = 80
      const items = Array.from({ length: N }, (_, i) => ({
        artist_name: `band-${i}`,
      }))
      mockCreate.mockResolvedValue(claudeTextResponse({ items }))

      let inFlight = 0
      let maxInFlight = 0
      // Gate every search briefly so concurrency actually overlaps; count the
      // peak number of simultaneously-in-flight searches.
      fetchConfig.searchHook = async () => {
        inFlight += 1
        maxInFlight = Math.max(maxInFlight, inFlight)
        await new Promise(r => setTimeout(r, 5))
        inFlight -= 1
      }

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      await readJson(res)

      expect(res.status).toBe(200)
      // The whole point of p-limit(20): the fan-out is bounded.
      expect(maxInFlight).toBeLessThanOrEqual(20)
      // Sanity: with 80 rows and a 5ms gate, we DID run in parallel (a purely
      // sequential loop would peak at 1).
      expect(maxInFlight).toBeGreaterThan(1)
    })

    it('degrades a single row whose search fails to "no match" without rejecting the batch', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          items: [
            { artist_name: 'GoodMatch' },
            { artist_name: 'BackendDown' }, // its /artists/search returns !ok
            { artist_name: 'AlsoGood' },
          ],
        })
      )
      fetchConfig.artistResults['GoodMatch'] = {
        artists: [{ id: 1, name: 'GoodMatch', slug: 'goodmatch' }],
        count: 1,
      }
      fetchConfig.artistResults['AlsoGood'] = {
        artists: [{ id: 3, name: 'AlsoGood', slug: 'alsogood' }],
        count: 1,
      }
      fetchConfig.searchFailFor = new Set(['BackendDown'])

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      const body = await readJson(res)

      // Whole batch still succeeds; the failed row is present but unmatched,
      // and order is preserved.
      expect(res.status).toBe(200)
      expect(body.data?.items).toHaveLength(3)
      expect(body.data?.items[0]).toMatchObject({
        artist_name: 'GoodMatch',
        matched_artist_id: 1,
      })
      expect(body.data?.items[1].artist_name).toBe('BackendDown')
      expect(body.data?.items[1].matched_artist_id).toBeUndefined()
      expect(body.data?.items[1].artist_suggestions).toBeUndefined()
      expect(body.data?.items[2]).toMatchObject({
        artist_name: 'AlsoGood',
        matched_artist_id: 3,
      })
    })
  })

  describe('rate limiting (PSY-855 — must stay intact)', () => {
    it('returns 429 and never calls Anthropic when throttled', async () => {
      fetchConfig.throttleBody = {
        allowed: false,
        retry_after_seconds: 2520,
        limit: 10,
        window_seconds: 3600,
      }

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))

      expect(res.status).toBe(429)
      expect(res.headers.get('Retry-After')).toBe('2520')
      expect(mockCreate).not.toHaveBeenCalled()
    })
  })

  describe('auth + config', () => {
    it('returns 401 with no auth cookie and never calls Anthropic', async () => {
      mockAuthCookie = undefined

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))

      expect(res.status).toBe(401)
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 503 + Sentry when ANTHROPIC_API_KEY is unset', async () => {
      delete process.env.ANTHROPIC_API_KEY

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      const body = await readJson(res)

      expect(res.status).toBe(503)
      expect(body.error).toBe('AI service not configured')
      expect(mockCaptureException).toHaveBeenCalledTimes(1)
    })
  })

  describe('extraction warnings', () => {
    it('warns when no items are found', async () => {
      mockCreate.mockResolvedValue(claudeTextResponse({ items: [] }))

      const res = await POST(makeRequest({ type: 'text', text: 'list' }))
      const body = await readJson(res)

      expect(res.status).toBe(200)
      expect(body.warnings).toContain('No items were found in the input')
    })

    it('caps the item array server-side and warns about truncation', async () => {
      // 251 rows in → capped to 250 with a truncation warning.
      const items = Array.from({ length: 251 }, (_, i) => ({
        artist_name: `cap-${i}`,
      }))
      mockCreate.mockResolvedValue(claudeTextResponse({ items }))

      const res = await POST(makeRequest({ type: 'text', text: 'huge list' }))
      const body = await readJson(res)

      expect(res.status).toBe(200)
      expect(body.data?.items).toHaveLength(250)
      expect(
        body.warnings?.some(w => w.includes('truncated to the first 250'))
      ).toBe(true)
    })
  })
})
