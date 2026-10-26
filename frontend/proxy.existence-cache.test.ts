import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { clearExistenceVerdictsForTests, proxy } from './proxy'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

function passedThrough(response: Response): boolean {
  return response.headers.get('x-middleware-next') === '1'
}

const FIVE_MINUTES_MS = 300_000

/**
 * Positive existence verdicts are remembered for five minutes; negative ones
 * never are.
 */
describe('proxy existence verdict cache', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-25T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    clearExistenceVerdictsForTests()
  })

  it('answers a second view inside five minutes without asking the backend', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(null, { status: 200 }))

    expect(passedThrough(await proxy(requestFor('/artists/cached-artist')))).toBe(true)
    vi.advanceTimersByTime(FIVE_MINUTES_MS - 1)
    expect(passedThrough(await proxy(requestFor('/artists/cached-artist')))).toBe(true)

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('asks again once the verdict has expired', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(null, { status: 200 }))

    await proxy(requestFor('/artists/expiring-artist'))
    vi.advanceTimersByTime(FIVE_MINUTES_MS)
    await proxy(requestFor('/artists/expiring-artist'))

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('asks about a 404 every time', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(null, { status: 404 }))

    expect((await proxy(requestFor('/artists/missing-artist'))).status).toBe(404)
    expect((await proxy(requestFor('/artists/missing-artist'))).status).toBe(404)

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('lets an entity created after a 404 through on the next view', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementationOnce(async () => new Response(null, { status: 404 }))
      .mockImplementationOnce(async () => new Response(null, { status: 200 }))

    expect((await proxy(requestFor('/artists/new-artist'))).status).toBe(404)
    expect(passedThrough(await proxy(requestFor('/artists/new-artist')))).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it.each([429, 500, 503])('does not remember a failed probe (%d)', async status => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(null, { status }))

    expect(passedThrough(await proxy(requestFor('/artists/flaky-artist')))).toBe(true)
    expect(passedThrough(await proxy(requestFor('/artists/flaky-artist')))).toBe(true)

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('does not remember a probe that threw', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockRejectedValueOnce(new TypeError('fetch failed'))
      .mockImplementationOnce(async () => new Response(null, { status: 404 }))

    expect(passedThrough(await proxy(requestFor('/artists/unreachable-artist')))).toBe(true)
    expect((await proxy(requestFor('/artists/unreachable-artist'))).status).toBe(404)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('keys verdicts by the probed path, so another entity is still asked', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(null, { status: 200 }))

    await proxy(requestFor('/artists/one-artist'))
    await proxy(requestFor('/venues/one-artist'))
    await proxy(requestFor('/artists/another-artist'))

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      'http://localhost:8080/entities/artists/one-artist/exists',
      'http://localhost:8080/entities/venues/one-artist/exists',
      'http://localhost:8080/entities/artists/another-artist/exists',
    ])
  })
})
