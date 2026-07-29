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
