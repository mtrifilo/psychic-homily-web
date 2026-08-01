import { beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException, captureMessage } = vi.hoisted(() => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
  captureMessage,
}))

import { FIRST_SCREEN_REVALIDATE_SECONDS, fetchListPayload } from './fetchListPayload'

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

const call = (fetchImpl: typeof fetch) =>
  fetchListPayload<{ venues: unknown[]; total: number }>({
    url: 'https://api.example.test/venues?limit=50',
    service: 'venues-first-screen',
    fetchImpl,
  })

describe('fetchListPayload', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns the WHOLE body, not just a collection', async () => {
    // The list components read `total` and `pagination` off the same object
    // they read rows from, so a helper that extracted one array would seed a
    // cache entry the hook cannot use.
    const body = { venues: [{ slug: 'the-rebel-lounge' }], total: 198 }
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse(body))

    await expect(call(fetchImpl)).resolves.toEqual(body)
    expect(fetchImpl).toHaveBeenCalledWith(
      'https://api.example.test/venues?limit=50',
      expect.objectContaining({
        next: { revalidate: FIRST_SCREEN_REVALIDATE_SECONDS },
        signal: expect.any(AbortSignal),
      }),
    )
    expect(captureMessage).not.toHaveBeenCalled()
    expect(captureException).not.toHaveBeenCalled()
  })

  it('distinguishes a genuinely empty catalogue from a failure', async () => {
    // This is the whole reason the helper returns `T | null`. An empty list is
    // an answer and must survive as one; a 500 is not an answer and must not
    // be renderable as "No upcoming shows at this time."
    const empty = { venues: [], total: 0 }
    await expect(
      call(vi.fn().mockResolvedValue(jsonResponse(empty))),
    ).resolves.toEqual(empty)

    await expect(
      call(vi.fn().mockResolvedValue(new Response(null, { status: 500 }))),
    ).resolves.toBeNull()
  })

  it('reports a 4xx as well as a 5xx', async () => {
    // Both ends are ours: a 4xx here is a defect in our own request, not an
    // outage, and treating it as unremarkable is how `/venues?limit=200`
    // silently dropped its ItemList in production for months.
    const fetchImpl = vi.fn().mockResolvedValue(new Response(null, { status: 422 }))

    await expect(call(fetchImpl)).resolves.toBeNull()
    expect(captureMessage).toHaveBeenCalledWith(
      'venues-first-screen: API returned 422',
      expect.objectContaining({ level: 'error' }),
    )
  })

  it('returns null and reports when the request throws', async () => {
    const fetchImpl = vi.fn().mockRejectedValue(new Error('ECONNREFUSED'))

    await expect(call(fetchImpl)).resolves.toBeNull()
    expect(captureException).toHaveBeenCalled()
  })

  it('returns null and reports on a 200 whose body is not an object', async () => {
    // A contract break, not an empty list — reporting it keeps a backend
    // shape change from surfacing as a blank page nobody investigates.
    for (const body of [null, [1, 2], 'nope']) {
      captureMessage.mockClear()
      await expect(
        call(vi.fn().mockResolvedValue(jsonResponse(body))),
      ).resolves.toBeNull()
      expect(captureMessage).toHaveBeenCalledWith(
        'venues-first-screen: response body is not an object',
        expect.objectContaining({ level: 'error' }),
      )
    }
  })
})
