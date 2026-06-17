import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NextRequest } from 'next/server'
import { cookies } from 'next/headers'
import Anthropic from '@anthropic-ai/sdk'
import { POST } from './route'

vi.mock('next/headers', () => ({ cookies: vi.fn() }))

// Mock the SDK constructor (so messages.create is controllable) but keep the
// REAL error classes so the route's `instanceof Anthropic.APIError` /
// `Anthropic.APIConnectionTimeoutError` checks resolve. Those statics are
// inherited from BaseAnthropic (not own-enumerable on Anthropic), so copy them
// by setting the prototype chain rather than Object.assign.
const mockCreate = vi.fn()
vi.mock('@anthropic-ai/sdk', async (importActual) => {
  const actual = await importActual<typeof import('@anthropic-ai/sdk')>()
  // Must be a real function (not an arrow) so `new Anthropic(...)` works.
  const MockAnthropic = vi.fn(function (this: { messages: unknown }) {
    this.messages = { create: mockCreate }
  })
  Object.setPrototypeOf(MockAnthropic, actual.default)
  return { ...actual, default: MockAnthropic }
})

const mockCookies = vi.mocked(cookies)
const BACKEND = 'http://localhost:8080'
const ARTIST_ID = '42'

function setAuthToken(token?: string) {
  mockCookies.mockResolvedValue({
    get: (name: string) =>
      name === 'auth_token' && token ? { name, value: token } : undefined,
  } as unknown as Awaited<ReturnType<typeof cookies>>)
}

function mockBackendFetch() {
  return vi
    .spyOn(globalThis, 'fetch')
    .mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === `${BACKEND}/auth/profile`) {
        return new Response(
          JSON.stringify({ success: true, user: { id: '1', is_admin: true } }),
          { status: 200 }
        )
      }
      if (url === `${BACKEND}/artists/${ARTIST_ID}`) {
        return new Response(JSON.stringify({ id: 42, name: 'Soroche' }), {
          status: 200,
        })
      }
      throw new Error(`Unexpected fetch in test: ${url}`)
    })
}

const postReq = () =>
  new NextRequest(
    `http://localhost:3000/api/admin/artists/${ARTIST_ID}/discover-music`,
    { method: 'POST' }
  )
const params = { params: Promise.resolve({ id: ARTIST_ID }) }

// Resolve the model call with a single text block (the route concatenates
// text blocks then parses the JSON out of them).
function mockLLMText(text: string) {
  mockCreate.mockResolvedValueOnce({ content: [{ type: 'text', text }] })
}

// Build a real Anthropic.APIError instance without depending on the SDK's
// constructor signature (which varies across versions): set the prototype so
// the route's `instanceof Anthropic.APIError` checks resolve, then set the two
// fields the route reads (`status`, `message`).
function apiError(status: number, message: string): Error {
  const e = Object.create(Anthropic.APIError.prototype) as Error &
    Record<string, unknown>
  e.status = status
  e.message = message
  return e
}

// A valid candidate of each platform shape the route keeps.
const BC_ALBUM = 'https://soroche.bandcamp.com/album/whispers'
const BC_TRACK = 'https://soroche.bandcamp.com/track/lone-single'
const SP_ARTIST = 'https://open.spotify.com/artist/0OdUWJ0sBjDrqHygGUXeCF'

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubEnv('ANTHROPIC_API_KEY', 'test-key')
  setAuthToken('admin-token')
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
})

describe('POST /api/admin/artists/[id]/discover-music', () => {
  it('maps an Anthropic connection timeout to a clean 504 TIMEOUT', async () => {
    mockBackendFetch()
    mockCreate.mockRejectedValueOnce(new Anthropic.APIConnectionTimeoutError())

    const res = await POST(postReq(), params)

    expect(res.status).toBe(504)
    expect((await res.json()).error).toBe('TIMEOUT')
  })

  it('calls the model as a single bounded attempt (timeout + maxRetries:0)', async () => {
    // maxRetries:0 is load-bearing: with the SDK default (2) a timeout retries
    // past maxDuration and the platform 504s before TIMEOUT can be returned.
    mockBackendFetch()
    mockCreate.mockResolvedValueOnce({
      content: [{ type: 'text', text: '{"bandcamp":[],"spotify":[]}' }],
    })

    const res = await POST(postReq(), params)

    expect(res.status).toBe(200)
    expect(mockCreate).toHaveBeenCalledWith(expect.anything(), {
      timeout: 55_000,
      maxRetries: 0,
    })
  })

  describe('candidate parsing + normalization', () => {
    it('normalizes candidate fields: keeps text, defaults bad confidence to low, empties to null', async () => {
      mockBackendFetch()
      mockLLMText(
        JSON.stringify({
          bandcamp: [
            {
              url: BC_ALBUM,
              name_as_listed: 'Soroche',
              location: '',
              notable_release: 'Whispers',
              genres: null,
              popularity: '',
              confidence: 'bogus',
              why_might_match: 'same hometown',
            },
          ],
          spotify: [],
        })
      )

      const res = await POST(postReq(), params)
      const body = await res.json()

      expect(res.status).toBe(200)
      expect(body.bandcamp).toHaveLength(1)
      expect(body.bandcamp[0]).toEqual({
        url: BC_ALBUM,
        name_as_listed: 'Soroche',
        location: null, // empty string → null
        notable_release: 'Whispers',
        genres: null,
        popularity: null, // empty string → null
        confidence: 'low', // unknown value → 'low'
        why_might_match: 'same hometown',
      })
    })

    it('strips a ```json code fence with preamble prose', async () => {
      mockBackendFetch()
      mockLLMText(
        'Here are the candidates I found:\n```json\n' +
          JSON.stringify({ bandcamp: [{ url: BC_ALBUM }], spotify: [] }) +
          '\n```\nHope that helps!'
      )

      const res = await POST(postReq(), params)
      const body = await res.json()

      expect(res.status).toBe(200)
      expect(body.bandcamp.map((c: { url: string }) => c.url)).toEqual([BC_ALBUM])
    })

    it('returns PARSE_FAILED (502) when the response has no JSON object', async () => {
      mockBackendFetch()
      mockLLMText('Sorry, I could not find anything for that artist.')

      const res = await POST(postReq(), params)

      expect(res.status).toBe(502)
      expect((await res.json()).error).toBe('PARSE_FAILED')
    })
  })

  describe('platform URL-shape filtering', () => {
    it('drops Bandcamp profile URLs but keeps /album/ and /track/ URLs', async () => {
      mockBackendFetch()
      mockLLMText(
        JSON.stringify({
          bandcamp: [
            { url: 'https://soroche.bandcamp.com' }, // profile root → dropped
            { url: 'https://soroche.bandcamp.com/music' }, // profile → dropped
            { url: BC_ALBUM }, // kept
            { url: BC_TRACK }, // kept
          ],
          spotify: [],
        })
      )

      const res = await POST(postReq(), params)
      const body = await res.json()

      expect(res.status).toBe(200)
      expect(body.bandcamp.map((c: { url: string }) => c.url)).toEqual([
        BC_ALBUM,
        BC_TRACK,
      ])
    })

    it('keeps a Spotify artist URL with a ?si= suffix and drops non-artist URLs', async () => {
      mockBackendFetch()
      mockLLMText(
        JSON.stringify({
          bandcamp: [],
          spotify: [
            { url: `${SP_ARTIST}?si=abc123` }, // kept (the ?si= drop bug, PSY-1108)
            { url: 'https://open.spotify.com/playlist/37i9dQZF1DX' }, // dropped
            { url: 'https://open.spotify.com/album/1DFixLWuPkv3KT3TnV35m3' }, // dropped
          ],
        })
      )

      const res = await POST(postReq(), params)
      const body = await res.json()

      expect(res.status).toBe(200)
      expect(body.spotify.map((c: { url: string }) => c.url)).toEqual([
        `${SP_ARTIST}?si=abc123`,
      ])
    })

    it('drops candidates with a missing or empty url', async () => {
      mockBackendFetch()
      mockLLMText(
        JSON.stringify({
          bandcamp: [{ name_as_listed: 'no url here' }, { url: '' }, { url: BC_ALBUM }],
          spotify: [],
        })
      )

      const res = await POST(postReq(), params)
      const body = await res.json()

      expect(res.status).toBe(200)
      expect(body.bandcamp.map((c: { url: string }) => c.url)).toEqual([BC_ALBUM])
    })

    it('dedupes repeated URLs so picker React keys stay unique', async () => {
      mockBackendFetch()
      mockLLMText(
        JSON.stringify({
          bandcamp: [{ url: BC_ALBUM }, { url: BC_ALBUM }],
          spotify: [{ url: SP_ARTIST }, { url: SP_ARTIST }],
        })
      )

      const res = await POST(postReq(), params)
      const body = await res.json()

      expect(res.status).toBe(200)
      expect(body.bandcamp).toHaveLength(1)
      expect(body.spotify).toHaveLength(1)
    })
  })

  describe('error taxonomy', () => {
    it('maps an Anthropic 429 to a clean 429 RATE_LIMIT', async () => {
      mockBackendFetch()
      mockCreate.mockRejectedValueOnce(apiError(429, 'Too Many Requests'))

      const res = await POST(postReq(), params)

      expect(res.status).toBe(429)
      expect((await res.json()).error).toBe('RATE_LIMIT')
    })

    it('maps a credits/billing APIError to a 503 API_CREDITS_EXHAUSTED', async () => {
      mockBackendFetch()
      mockCreate.mockRejectedValueOnce(
        apiError(400, 'Your credit balance is too low to access the API')
      )

      const res = await POST(postReq(), params)

      expect(res.status).toBe(503)
      expect((await res.json()).error).toBe('API_CREDITS_EXHAUSTED')
    })
  })

  describe('auth + config gates', () => {
    it('returns 401 when no auth cookie is present', async () => {
      setAuthToken(undefined)

      const res = await POST(postReq(), params)

      expect(res.status).toBe(401)
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 403 when the user is not an admin', async () => {
      vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
        const url = String(input)
        if (url === `${BACKEND}/auth/profile`) {
          return new Response(
            JSON.stringify({ success: true, user: { id: '1', is_admin: false } }),
            { status: 200 }
          )
        }
        throw new Error(`Unexpected fetch in test: ${url}`)
      })

      const res = await POST(postReq(), params)

      expect(res.status).toBe(403)
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('returns 503 when ANTHROPIC_API_KEY is not configured', async () => {
      vi.stubEnv('ANTHROPIC_API_KEY', '')
      mockBackendFetch()

      const res = await POST(postReq(), params)

      expect(res.status).toBe(503)
      expect((await res.json()).error).toBe('AI service not configured')
      expect(mockCreate).not.toHaveBeenCalled()
    })
  })
})
