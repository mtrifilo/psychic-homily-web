import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// Deliberately NOT mocking '@/features/artists/api': the URL and the key this
// hook lands on are the whole subject.
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import { useArtistGraphCard } from './useArtistGraphCard'

/**
 * The graph card used to build its own query string, appending the BROWSER's
 * zone whenever `window` existed. That parameter is inert — the handler makes
 * the next-show cutoff in the show's own venue-local day and documents the
 * param as deprecated and ignored — so it bought nothing and made the request
 * differ between a server render and the browser's.
 *
 * Unlike the shows-list hooks, nothing here is guarded by a type: this hook
 * assembles its own URL, so only an assertion on that URL can keep a zone (or
 * any other per-viewer input) from being reintroduced inside the queryFn.
 */
describe('useArtistGraphCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('requests the bare graph-card endpoint, with no query string', async () => {
    mockApiRequest.mockResolvedValue({ id: 7, name: 'Gatecreeper' })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useArtistGraphCard({ artistId: 7 }), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      artistEndpoints.GRAPH_CARD(7),
      { method: 'GET' },
    )
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).not.toContain('?')
    expect(url).not.toContain('timezone')
  })

  it('sends nothing derived from the viewer, in any environment', async () => {
    mockApiRequest.mockResolvedValue({ id: 7 })
    const viewerZone = Intl.DateTimeFormat().resolvedOptions().timeZone

    const { result } = renderHook(() => useArtistGraphCard({ artistId: 7 }), {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).not.toContain(viewerZone)
    expect(url).not.toContain(encodeURIComponent(viewerZone))
  })

  it('resolves a slug the same way it resolves a numeric id', async () => {
    // Mixed-type graph surfaces pass a node slug rather than an artist id; the
    // handler resolves either, so the hook must not special-case one.
    mockApiRequest.mockResolvedValue({ slug: 'gatecreeper' })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () => useArtistGraphCard({ artistId: 'gatecreeper' }),
      { wrapper: createWrapperWithClient(queryClient) },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      artistEndpoints.GRAPH_CARD('gatecreeper'),
      { method: 'GET' },
    )
    expect(queryClient.getQueryCache().getAll()[0].queryHash).toBe(
      hashKey(artistQueryKeys.graphCard('gatecreeper')),
    )
  })

  it('issues nothing until it has an identity to ask about', async () => {
    mockApiRequest.mockResolvedValue({})

    for (const artistId of [null, 0, ''] as const) {
      const { result } = renderHook(() => useArtistGraphCard({ artistId }), {
        wrapper: createWrapperWithClient(createTestQueryClient()),
      })
      expect(result.current.fetchStatus).toBe('idle')
    }
    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})
