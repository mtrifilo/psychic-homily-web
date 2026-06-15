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
})
