import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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
    collection: 'venues',
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

  // A shape-drifted 200 must not reach the query cache. For venues and scenes
  // it would render as the empty state; for shows it is worse, because
  // `ShowList` reads `data?.pagination.has_more` and a seeded object without
  // `pagination` throws during render.
  it('returns null and reports when the collection key is missing or not an array', async () => {
    for (const body of [{ total: 0 }, { venues: 'nope', total: 0 }]) {
      captureMessage.mockClear()
      await expect(
        call(vi.fn().mockResolvedValue(jsonResponse(body))),
      ).resolves.toBeNull()
      expect(captureMessage).toHaveBeenCalledWith(
        'venues-first-screen: response has no "venues" array',
        expect.objectContaining({ level: 'error' }),
      )
    }
  })

  // The Go handlers marshal a nil element of a `[]*Response` slice as `null`,
  // and `app/shows/page.tsx` dereferences `.slug` OUTSIDE any try block — so
  // one null row 500s the route with no Sentry event from here, and reaches
  // `ShowCard` through the seeded cache. `fetchSeoList` guarded this; the
  // helper that replaced it on /shows has to keep guarding it.
  it('drops null rows rather than seeding them, and reports that it did', async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ venues: [{ slug: 'a' }, null, { slug: 'b' }], total: 3 }),
      )

    await expect(call(fetchImpl)).resolves.toEqual({
      venues: [{ slug: 'a' }, { slug: 'b' }],
      total: 3,
    })
    expect(captureMessage).toHaveBeenCalledWith(
      'venues-first-screen: "venues" contained 1 null row(s)',
      expect.objectContaining({ level: 'error' }),
    )
  })

  it('leaves a clean payload untouched and reports nothing', async () => {
    const body = { venues: [{ slug: 'a' }], total: 1 }

    await expect(
      call(vi.fn().mockResolvedValue(jsonResponse(body))),
    ).resolves.toEqual(body)
    expect(captureMessage).not.toHaveBeenCalled()
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

// PSY-1674. Same invariant as lib/seo/fetchSeoList.test.ts: this helper degrades
// to `null` on every other error, and a build-time budget breach is the one
// thing it must NOT absorb. Swallowing it would leave the build green with a
// route that has silently stopped caching.
describe('fetchListPayload and the Data Cache budget gate', () => {
  const originalPhase = process.env.NEXT_PHASE

  afterEach(() => {
    if (originalPhase === undefined) delete process.env.NEXT_PHASE
    else process.env.NEXT_PHASE = originalPhase
  })

  const oversized = () =>
    jsonResponse({ venues: [{ slug: 'a', pad: 'x'.repeat(2_200_000) }], total: 1 })

  it('rethrows a budget breach during a build rather than seeding null', async () => {
    process.env.NEXT_PHASE = 'phase-production-build'
    vi.spyOn(console, 'warn').mockImplementation(() => {})

    await expect(call(vi.fn().mockResolvedValue(oversized()))).rejects.toThrow(
      /Data Cache budget exceeded/
    )
    expect(captureException).not.toHaveBeenCalled()
  })

  it('still degrades to a usable payload at request time', async () => {
    delete process.env.NEXT_PHASE

    const result = await call(vi.fn().mockResolvedValue(oversized()))
    expect(result?.total).toBe(1)
    expect(captureMessage).toHaveBeenCalledWith(
      expect.stringContaining('data-cache-budget'),
      expect.objectContaining({ level: 'error' })
    )
  })
})
