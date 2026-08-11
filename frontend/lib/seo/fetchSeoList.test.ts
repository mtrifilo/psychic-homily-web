import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException, captureMessage } = vi.hoisted(() => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
  captureMessage,
}))

import { BUILD_TIME_API_FETCH_TIMEOUT_MS } from '@/lib/build-time-api'
import { SEO_LIST_REVALIDATE_SECONDS, fetchSeoList } from './fetchSeoList'

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

const call = (fetchImpl: typeof fetch, timeoutMs?: number) =>
  fetchSeoList<{ slug: string }>({
    url: 'https://api.example.test/venues?limit=100',
    collection: 'venues',
    service: 'venues-listing',
    fetchImpl,
    ...(timeoutMs === undefined ? {} : { timeoutMs }),
  })

describe('fetchSeoList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns the named collection from a successful response', async () => {
    const venues = [{ slug: 'the-rebel-lounge' }]
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ venues }))

    await expect(call(fetchImpl)).resolves.toEqual(venues)
    expect(fetchImpl).toHaveBeenCalledWith(
      'https://api.example.test/venues?limit=100',
      expect.objectContaining({
        next: { revalidate: SEO_LIST_REVALIDATE_SECONDS },
        signal: expect.any(AbortSignal),
      })
    )
    expect(captureMessage).not.toHaveBeenCalled()
    expect(captureException).not.toHaveBeenCalled()
  })

  // The regression this helper exists for: `/venues?limit=200` 422'd on every
  // render because the endpoint caps `limit` at 100, and the old per-page
  // fail-open only reported 5xx — so the missing ItemList was invisible.
  it('reports a 4xx, not just a 5xx, and still fails open', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response(null, { status: 422 }))

    await expect(call(fetchImpl)).resolves.toEqual([])
    expect(captureMessage).toHaveBeenCalledWith(
      'venues-listing: API returned 422',
      expect.objectContaining({
        level: 'error',
        tags: { service: 'venues-listing' },
      })
    )
  })

  it('reports a 5xx and fails open', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response(null, { status: 503 }))

    await expect(call(fetchImpl)).resolves.toEqual([])
    expect(captureMessage).toHaveBeenCalledWith(
      'venues-listing: API returned 503',
      expect.objectContaining({ tags: { service: 'venues-listing' } })
    )
  })

  // Array.isArray admits [null], and every caller dereferences item.slug
  // outside this helper's try block — one null element would 500 the page with
  // no Sentry event, which is the opposite of the fail-open this helper promises.
  it('drops null elements rather than handing callers something to deref', async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ venues: [null, { slug: 'a' }, undefined] }))

    await expect(call(fetchImpl)).resolves.toEqual([{ slug: 'a' }])
  })

  it('treats a 200 without the collection array as a contract break', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ venues: null }))

    await expect(call(fetchImpl)).resolves.toEqual([])
    expect(captureMessage).toHaveBeenCalledWith(
      'venues-listing: response has no "venues" array',
      expect.objectContaining({ tags: { service: 'venues-listing' } })
    )
  })

  it('reports a thrown transport error and fails open', async () => {
    const error = new TypeError('fetch failed')
    const fetchImpl = vi.fn().mockRejectedValue(error)

    await expect(call(fetchImpl)).resolves.toEqual([])
    expect(captureException).toHaveBeenCalledWith(
      error,
      expect.objectContaining({ tags: { service: 'venues-listing' } })
    )
  })

  it('defaults to the shared build-time budget and honours an override', async () => {
    const timeoutSpy = vi
      .spyOn(AbortSignal, 'timeout')
      .mockReturnValue(new AbortController().signal)
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ venues: [] }))

    await call(fetchImpl)
    expect(timeoutSpy).toHaveBeenLastCalledWith(BUILD_TIME_API_FETCH_TIMEOUT_MS)

    await call(fetchImpl, 30_000)
    expect(timeoutSpy).toHaveBeenLastCalledWith(30_000)
  })

  it('fails open when the timeout aborts the request', async () => {
    const controller = new AbortController()
    vi.spyOn(AbortSignal, 'timeout').mockReturnValue(controller.signal)
    const timeoutError = new DOMException(
      'The operation was aborted due to timeout',
      'TimeoutError'
    )
    const fetchImpl = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(timeoutError))
        })
    )

    const result = call(fetchImpl)
    controller.abort(timeoutError)

    await expect(result).resolves.toEqual([])
    expect(captureException).toHaveBeenCalledWith(
      timeoutError,
      expect.objectContaining({ tags: { service: 'venues-listing' } })
    )
  })
})

// PSY-1764. The truncation that replaced the 422 above: the call asked for a
// page, the response carried `total`, the call discarded it, and the ItemList
// advertised a fraction of the catalogue while every available signal said
// healthy. `/venues` no longer asks for a page at all, so these tests guard the
// reachable cause on a complete list (rows that cannot form a URL) and the next
// caller pointed at a paginated endpoint.
describe('fetchSeoList and a short list', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reports a list shorter than the total the response reports', async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ venues: [{ slug: 'a' }], total: 8 }))

    // Still returns what it got: a partial ItemList beats none, exactly as a
    // failed fetch still renders the page. The event is what changes.
    await expect(call(fetchImpl)).resolves.toEqual([{ slug: 'a' }])
    expect(captureMessage).toHaveBeenCalledWith(
      expect.stringContaining('venues-listing: list is short of the total'),
      expect.objectContaining({
        // WARNING, not error: on a limitless endpoint the reachable cause is a
        // venue with no slug, which no deploy fixes and which would otherwise
        // re-page someone hourly forever — and would do it in the same Sentry
        // issue a real truncation lands in.
        level: 'warning',
        tags: { service: 'venues-listing' },
        extra: expect.objectContaining({ received: 1, total: 8, missing: 7 }),
      })
    )
  })

  // The numbers ride in `extra` so Sentry groups every revalidation into one
  // issue. A message carrying the counts would open a new issue on each
  // catalogue change, which is how a real signal becomes noise and gets muted.
  it('keeps the counts out of the message so the events group', async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ venues: [{ slug: 'a' }], total: 8 }))

    await call(fetchImpl)
    expect(captureMessage.mock.calls[0][0]).not.toMatch(/\d/)
  })

  it('says nothing when the list covers the reported total', async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ venues: [{ slug: 'a' }, { slug: 'b' }], total: 2 }))

    await expect(call(fetchImpl)).resolves.toHaveLength(2)
    expect(captureMessage).not.toHaveBeenCalled()
  })

  // A projection endpoint that reports no total is not claiming a larger set, so
  // absence must read as "unknown", never as a total of zero — which would make
  // every such response look complete OR make an empty one look short, depending
  // on which way the comparison was written.
  it('says nothing when the response reports no total at all', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ venues: [{ slug: 'a' }] }))

    await expect(call(fetchImpl)).resolves.toHaveLength(1)
    expect(captureMessage).not.toHaveBeenCalled()
  })

  // Counted AFTER the null filter above, because a null element is not an entry
  // the ItemList can advertise either. Two venues, one of them null, against a
  // total of two is a shortfall of one.
  it('counts what the caller can actually use, not what the array held', async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ venues: [null, { slug: 'a' }], total: 2 }))

    await expect(call(fetchImpl)).resolves.toEqual([{ slug: 'a' }])
    expect(captureMessage).toHaveBeenCalledWith(
      expect.stringContaining('list is short of the total'),
      expect.objectContaining({ extra: expect.objectContaining({ received: 1, total: 2 }) })
    )
  })
})

// PSY-1674. This helper fails open on every other error by design, and the ONE
// exception is what the Data Cache budget gate is built on: a build-time budget
// breach must ESCAPE the catch, because absorbing it turns the gate into a
// no-op and restores exactly the silent cache failure it exists to remove.
// Without these two tests, deleting the
// `if (error instanceof DataCacheBudgetError) throw error` line leaves every
// other test in this file passing.
describe('fetchSeoList and the Data Cache budget gate', () => {
  const originalPhase = process.env.NEXT_PHASE

  afterEach(() => {
    if (originalPhase === undefined) delete process.env.NEXT_PHASE
    else process.env.NEXT_PHASE = originalPhase
  })

  // Comfortably past the ~1.5 MiB raw budget.
  const oversized = () =>
    jsonResponse({ venues: [{ slug: 'a', pad: 'x'.repeat(2_200_000) }] })

  it('rethrows a budget breach during a build instead of failing open', async () => {
    process.env.NEXT_PHASE = 'phase-production-build'
    vi.spyOn(console, 'warn').mockImplementation(() => {})

    await expect(call(vi.fn().mockResolvedValue(oversized()))).rejects.toThrow(
      /Data Cache budget exceeded/
    )
    // Fail-open would have swallowed it into an empty list plus a Sentry event.
    expect(captureException).not.toHaveBeenCalled()
  })

  it('still fails open at request time, where a rendered page beats a cache entry', async () => {
    delete process.env.NEXT_PHASE

    await expect(call(vi.fn().mockResolvedValue(oversized()))).resolves.toHaveLength(1)
    expect(captureMessage).toHaveBeenCalledWith(
      expect.stringContaining('data-cache-budget'),
      expect.objectContaining({ level: 'error' })
    )
  })
})
