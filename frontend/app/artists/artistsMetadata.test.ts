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

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

describe('getArtistsForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns artists from a successful response', async () => {
    const artists = [{ name: 'Desert Static', slug: 'desert-static' }]
    const fetchArtists = vi.fn().mockResolvedValue(jsonResponse({ artists }))

    await expect(getArtistsForMetadata(fetchArtists)).resolves.toEqual(artists)
    expect(fetchArtists).toHaveBeenCalledWith(
      expect.stringMatching(/\/artists$/),
      expect.objectContaining({
        next: { revalidate: 3600 },
        signal: expect.any(AbortSignal),
      })
    )
  })

  // The shared 10s build-time budget is deliberately overridden here: `/artists`
  // is unpaginated and has no `limit` parameter to bound it with.
  it('asks for a thirty-second budget, not the shared ten-second one', async () => {
    const timeoutSpy = vi
      .spyOn(AbortSignal, 'timeout')
      .mockReturnValue(new AbortController().signal)
    const fetchArtists = vi.fn().mockResolvedValue(jsonResponse({ artists: [] }))

    await getArtistsForMetadata(fetchArtists)
    expect(timeoutSpy).toHaveBeenCalledWith(30_000)
  })

  it('reports server errors and returns the fallback', async () => {
    const fetchArtists = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 503 }))

    await expect(getArtistsForMetadata(fetchArtists)).resolves.toEqual([])
    expect(captureMessage).toHaveBeenCalledWith(
      'artists-listing: API returned 503',
      expect.objectContaining({ tags: { service: 'artists-listing' } })
    )
  })

  it('reports a thrown fetch error and returns the fallback', async () => {
    const error = new TypeError('fetch failed')
    const fetchArtists = vi.fn().mockRejectedValue(error)

    await expect(getArtistsForMetadata(fetchArtists)).resolves.toEqual([])
    expect(captureException).toHaveBeenCalledWith(
      error,
      expect.objectContaining({ tags: { service: 'artists-listing' } })
    )
  })
})
