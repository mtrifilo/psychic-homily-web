import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException, captureMessage } = vi.hoisted(() => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
  captureMessage,
}))

import { getArtistsForMetadata } from './artistsMetadata'

describe('getArtistsForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns artists from a successful response', async () => {
    const artists = [{ name: 'Desert Static', slug: 'desert-static' }]
    const fetchArtists = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ artists }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    )

    await expect(getArtistsForMetadata(fetchArtists)).resolves.toEqual(artists)
    expect(fetchArtists).toHaveBeenCalledWith(
      expect.stringMatching(/\/artists$/),
      expect.objectContaining({
        next: { revalidate: 3600 },
        signal: expect.any(AbortSignal),
      })
    )
  })

  it('uses a ten-second timeout signal and falls back when it aborts', async () => {
    const controller = new AbortController()
    const timeoutSpy = vi
      .spyOn(AbortSignal, 'timeout')
      .mockReturnValue(controller.signal)
    const timeoutError = new DOMException(
      'The operation was aborted due to timeout',
      'TimeoutError'
    )
    const fetchArtists = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(timeoutError))
        })
    )

    const result = getArtistsForMetadata(fetchArtists)
    controller.abort(timeoutError)

    await expect(result).resolves.toEqual([])
    expect(timeoutSpy).toHaveBeenCalledWith(10_000)
    expect(captureException).toHaveBeenCalledWith(
      timeoutError,
      expect.objectContaining({ tags: { service: 'artists-listing' } })
    )
  })

  it('reports server errors and returns the fallback', async () => {
    const fetchArtists = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 503 }))

    await expect(getArtistsForMetadata(fetchArtists)).resolves.toEqual([])
    expect(captureMessage).toHaveBeenCalledWith(
      'Artists listing: API returned 503',
      expect.objectContaining({ tags: { service: 'artists-listing' } })
    )
  })
})
