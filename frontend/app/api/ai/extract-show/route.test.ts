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
// The route does `import Anthropic from '@anthropic-ai/sdk'`, then:
//   - `new Anthropic({ apiKey })`
//   - `anthropic.messages.create(...)`
//   - `error instanceof Anthropic.APIError`
//
// So the mocked default export must be a class whose instances expose a
// `messages.create` we control, AND must carry a real `APIError` static
// class so the route's `instanceof` branch is reachable. We model APIError
// off the real SDK shape (constructor: status, error, message, headers;
// extends Error so `.message` works) — see node_modules/@anthropic-ai/sdk/
// core/error.d.ts.
//
// vi.hoisted lets these definitions be referenced from the hoisted vi.mock
// factory AND from test bodies (e.g. constructing a MockAPIError to throw).
const { mockCreate, MockAPIError, MockAnthropic } = vi.hoisted(() => {
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
    MockAPIError: HoistedAPIError,
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
}))

// --- next/headers cookies mock --------------------------------------------
//
// `cookieStore.get('auth_token')` returns `{ value } | undefined`.
let mockAuthCookie: { value: string } | undefined = { value: 'valid-token' }
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({
    get: (name: string) =>
      name === 'auth_token' ? mockAuthCookie : undefined,
  })),
}))

// Import AFTER mocks are registered.
import { POST } from './route'
import type {
  ExtractShowRequest,
  ExtractShowResponse,
} from '@/lib/types/extraction'

// --- fetch router ---------------------------------------------------------
//
// The route calls fetch for three backend endpoints. Each test configures
// the desired responses; the router dispatches by URL substring.
interface FetchConfig {
  profileOk: boolean
  profileBody: unknown
  artistResults: Record<string, unknown>
  venueResults: Record<string, unknown>
}

let fetchConfig: FetchConfig

function makeResponse(ok: boolean, body: unknown): Response {
  return {
    ok,
    json: async () => body,
  } as unknown as Response
}

function installFetch() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()

    if (url.includes('/auth/profile')) {
      return makeResponse(fetchConfig.profileOk, fetchConfig.profileBody)
    }
    if (url.includes('/artists/search')) {
      const q = new URL(url).searchParams.get('q') ?? ''
      const body = fetchConfig.artistResults[q] ?? { artists: [], count: 0 }
      return makeResponse(true, body)
    }
    if (url.includes('/venues/search')) {
      const q = new URL(url).searchParams.get('q') ?? ''
      const body = fetchConfig.venueResults[q] ?? { venues: [], count: 0 }
      return makeResponse(true, body)
    }
    throw new Error(`Unexpected fetch URL in test: ${url}`)
  })
  vi.stubGlobal('fetch', fetchMock)
}

// Build a Claude response in the canonical SDK shape.
function claudeTextResponse(payload: unknown) {
  return {
    content: [{ type: 'text', text: JSON.stringify(payload) }],
  }
}

function makeRequest(body: ExtractShowRequest | string) {
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
    artistResults: {},
    venueResults: {},
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

async function readJson(res: Response): Promise<ExtractShowResponse> {
  return (await res.json()) as ExtractShowResponse
}

describe('POST /api/ai/extract-show', () => {
  describe('authentication', () => {
    it('returns 401 when no auth cookie is present', async () => {
      mockAuthCookie = undefined

      const res = await POST(makeRequest({ type: 'text', text: 'show flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(401)
      expect(body.success).toBe(false)
      expect(body.error).toBe('Authentication required')
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 401 when the auth cookie fails profile verification', async () => {
      fetchConfig.profileOk = false
      fetchConfig.profileBody = { success: false }

      const res = await POST(makeRequest({ type: 'text', text: 'show flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(401)
      expect(body.error).toBe('Authentication required')
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 401 when profile responds ok but success is false', async () => {
      fetchConfig.profileOk = true
      fetchConfig.profileBody = { success: false }

      const res = await POST(makeRequest({ type: 'text', text: 'show flyer' }))

      expect(res.status).toBe(401)
      expect(mockCreate).not.toHaveBeenCalled()
    })
  })

  describe('request validation', () => {
    it('returns 400 for an unknown request type', async () => {
      const res = await POST(
        makeRequest({ type: 'audio' } as unknown as ExtractShowRequest)
      )
      const body = await readJson(res)

      expect(res.status).toBe(400)
      expect(body.error).toContain('Invalid request type')
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 400 for an invalid image media type', async () => {
      const res = await POST(
        makeRequest({
          type: 'image',
          image_data: 'base64data',
          media_type: 'image/tiff' as unknown as ExtractShowRequest['media_type'],
        })
      )
      const body = await readJson(res)

      expect(res.status).toBe(400)
      expect(body.error).toContain('Invalid image type')
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 400 when image type is missing image_data', async () => {
      const res = await POST(makeRequest({ type: 'image' }))
      const body = await readJson(res)

      expect(res.status).toBe(400)
      expect(body.error).toBe('Image data is required')
    })

    it('returns 400 when text content exceeds 10,000 characters', async () => {
      const res = await POST(
        makeRequest({ type: 'text', text: 'a'.repeat(10001) })
      )
      const body = await readJson(res)

      expect(res.status).toBe(400)
      expect(body.error).toContain('exceeds maximum length')
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 400 when text content is empty', async () => {
      const res = await POST(makeRequest({ type: 'text', text: '   ' }))
      const body = await readJson(res)

      expect(res.status).toBe(400)
      expect(body.error).toBe('Text content is required')
    })

    it('returns 400 for an unparseable request body', async () => {
      const res = await POST(makeRequest('not-json'))
      const body = await readJson(res)

      expect(res.status).toBe(400)
      expect(body.error).toBe('Invalid request body')
    })
  })

  describe('service configuration', () => {
    it('returns 503 and reports to Sentry when ANTHROPIC_API_KEY is unset', async () => {
      delete process.env.ANTHROPIC_API_KEY

      const res = await POST(makeRequest({ type: 'text', text: 'show flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(503)
      expect(body.error).toBe('AI service not configured')
      expect(mockCaptureException).toHaveBeenCalledTimes(1)
    })
  })

  describe('successful text extraction', () => {
    it('returns parsed artists, venue, date, time, cost, and ages', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          artists: [
            { name: 'Sleep', is_headliner: true },
            { name: 'Om', is_headliner: false },
          ],
          venue: { name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ' },
          date: '2026-06-01',
          time: '20:00',
          cost: '$20',
          ages: '21+',
        })
      )

      const res = await POST(
        makeRequest({ type: 'text', text: 'Sleep + Om at The Rebel Lounge' })
      )
      const body = await readJson(res)

      expect(res.status).toBe(200)
      expect(body.success).toBe(true)
      expect(body.data?.artists).toHaveLength(2)
      expect(body.data?.artists[0]).toMatchObject({
        name: 'Sleep',
        is_headliner: true,
      })
      expect(body.data?.venue?.name).toBe('The Rebel Lounge')
      expect(body.data?.date).toBe('2026-06-01')
      expect(body.data?.time).toBe('20:00')
      expect(body.data?.cost).toBe('$20')
      expect(body.data?.ages).toBe('21+')
    })

    it('parses JSON wrapped in a markdown code block', async () => {
      mockCreate.mockResolvedValue({
        content: [
          {
            type: 'text',
            text: '```json\n{"artists":[{"name":"Earth","is_headliner":true}]}\n```',
          },
        ],
      })

      const res = await POST(makeRequest({ type: 'text', text: 'Earth show' }))
      const body = await readJson(res)

      expect(res.status).toBe(200)
      expect(body.data?.artists[0].name).toBe('Earth')
    })

    it('warns when no artists are found in the input', async () => {
      mockCreate.mockResolvedValue(claudeTextResponse({ artists: [] }))

      const res = await POST(makeRequest({ type: 'text', text: 'no artists' }))
      const body = await readJson(res)

      expect(res.status).toBe(200)
      expect(body.warnings).toContain('No artists were found in the input')
    })
  })

  describe('database matching', () => {
    it('attaches matched_id when an artist exactly matches the database', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          artists: [
            { name: 'Sleep', is_headliner: true, instagram_handle: '@sleep' },
          ],
        })
      )
      fetchConfig.artistResults['Sleep'] = {
        artists: [{ id: 42, name: 'Sleep', slug: 'sleep' }],
        count: 1,
      }

      const res = await POST(makeRequest({ type: 'text', text: 'Sleep' }))
      const body = await readJson(res)

      const artist = body.data?.artists[0]
      expect(artist?.matched_id).toBe(42)
      expect(artist?.matched_name).toBe('Sleep')
      expect(artist?.matched_slug).toBe('sleep')
      // Instagram handle is cleared once an artist is matched.
      expect(artist?.instagram_handle).toBeUndefined()
    })

    it('returns suggestions when an artist is a near (non-exact) match', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          artists: [{ name: 'Sleepy', is_headliner: true }],
        })
      )
      fetchConfig.artistResults['Sleepy'] = {
        artists: [
          { id: 1, name: 'Sleep', slug: 'sleep' },
          { id: 2, name: 'Sleepers', slug: 'sleepers' },
        ],
        count: 2,
      }

      const res = await POST(makeRequest({ type: 'text', text: 'Sleepy' }))
      const body = await readJson(res)

      const artist = body.data?.artists[0]
      expect(artist?.matched_id).toBeUndefined()
      expect(artist?.suggestions).toHaveLength(2)
      expect(artist?.suggestions?.[0]).toMatchObject({ id: 1, name: 'Sleep' })
    })

    it('attaches matched_id and DB city/state when a venue exactly matches', async () => {
      mockCreate.mockResolvedValue(
        claudeTextResponse({
          artists: [{ name: 'Om', is_headliner: true }],
          venue: { name: 'The Rebel Lounge', city: 'phx', state: 'XX' },
        })
      )
      fetchConfig.venueResults['The Rebel Lounge'] = {
        venues: [
          {
            id: 7,
            name: 'The Rebel Lounge',
            slug: 'the-rebel-lounge',
            city: 'Phoenix',
            state: 'AZ',
          },
        ],
        count: 1,
      }

      const res = await POST(
        makeRequest({ type: 'text', text: 'Om at The Rebel Lounge' })
      )
      const body = await readJson(res)

      const venue = body.data?.venue
      expect(venue?.matched_id).toBe(7)
      expect(venue?.matched_slug).toBe('the-rebel-lounge')
      // Matched venue overrides extracted city/state with canonical DB values.
      expect(venue?.city).toBe('Phoenix')
      expect(venue?.state).toBe('AZ')
    })
  })

  describe('error handling', () => {
    it('returns 500 and reports to Sentry when the SDK throws a generic error', async () => {
      mockCreate.mockRejectedValue(new Error('network exploded'))

      const res = await POST(makeRequest({ type: 'text', text: 'flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(500)
      expect(body.success).toBe(false)
      expect(body.error).toBe('An unexpected error occurred. Please try again.')
      expect(mockCaptureException).toHaveBeenCalledTimes(1)
    })

    it('returns 503 and reports to Sentry on an Anthropic APIError', async () => {
      mockCreate.mockRejectedValue(
        new MockAPIError(500, undefined, 'internal server error', undefined)
      )

      const res = await POST(makeRequest({ type: 'text', text: 'flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(503)
      expect(body.error).toBe('AI service error. Please try again.')
      expect(mockCaptureException).toHaveBeenCalledTimes(1)
    })

    it('returns 503 with a billing-aware message on a credits APIError', async () => {
      mockCreate.mockRejectedValue(
        new MockAPIError(
          400,
          undefined,
          'Your credit balance is too low',
          undefined
        )
      )

      const res = await POST(makeRequest({ type: 'text', text: 'flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(503)
      expect(body.error).toBe(
        'AI service temporarily unavailable. Please try again later.'
      )
      expect(mockCaptureException).toHaveBeenCalledTimes(1)
    })

    it('returns 500 with a parse error and reports to Sentry when Claude returns non-JSON', async () => {
      mockCreate.mockResolvedValue({
        content: [{ type: 'text', text: 'Sorry, I could not read the flyer.' }],
      })

      const res = await POST(makeRequest({ type: 'text', text: 'flyer' }))
      const body = await readJson(res)

      expect(res.status).toBe(500)
      expect(body.error).toBe('Failed to parse AI response')
      expect(body.warnings).toBeDefined()
    })
  })
})
