/**
 * @vitest-environment jsdom
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

import type { LinkSuggestionEntry, LinkSuggestionListResult } from './types'

const mockUseLinkSuggestions = vi.fn()
const mockReviewMutationResult = vi.fn()

vi.mock('./useDiscoveryTriage', () => ({
  useLinkSuggestions: (params: unknown) => mockUseLinkSuggestions(params),
  useReviewLinkSuggestion: () => mockReviewMutationResult(),
}))

import { DiscoveryTriage } from './DiscoveryTriage'

function makeEntry(overrides: Partial<LinkSuggestionEntry> = {}): LinkSuggestionEntry {
  return {
    id: 1,
    artist_id: 101,
    artist_name: 'Faetooth',
    artist_slug: 'faetooth',
    platform: 'spotify',
    url: 'https://open.spotify.com/artist/abc123',
    source: 'musicbrainz',
    mb_artist_id: 'mb-uuid',
    mb_artist_name: 'Faetooth',
    confidence: 'high',
    region_match: true,
    live: true,
    notes: null,
    status: 'pending',
    created_at: '2026-06-23T12:00:00Z',
    ...overrides,
  }
}

function mockList(
  data: LinkSuggestionListResult | null,
  opts: Partial<{ isLoading: boolean; isError: boolean; error: Error | null }> = {}
) {
  mockUseLinkSuggestions.mockReturnValue({
    data,
    isLoading: opts.isLoading ?? false,
    isError: opts.isError ?? false,
    error: opts.error ?? null,
  })
}

/**
 * Stub the review mutation. `mutate` invokes the supplied callbacks so the
 * component's success/error branches run synchronously in tests.
 */
function stubReviewMutation(
  behavior: 'success' | { error: unknown } = 'success'
) {
  const mutate = vi.fn(
    (
      _vars: unknown,
      callbacks?: { onSuccess?: () => void; onError?: (e: unknown) => void }
    ) => {
      if (behavior === 'success') {
        callbacks?.onSuccess?.()
      } else {
        callbacks?.onError?.(behavior.error)
      }
    }
  )
  mockReviewMutationResult.mockReturnValue({ mutate, isPending: false })
  return mutate
}

describe('DiscoveryTriage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stubReviewMutation('success')
  })

  it('renders the empty state when no pending suggestions', () => {
    mockList({ suggestions: [], total: 0 })
    renderWithProviders(<DiscoveryTriage />)
    expect(screen.getByTestId('link-suggestion-empty')).toBeInTheDocument()
  })

  it('renders one row per suggestion with the artist + URL', () => {
    mockList({
      suggestions: [
        makeEntry({ id: 1, artist_name: 'Alpha' }),
        makeEntry({ id: 2, artist_name: 'Beta', artist_slug: 'beta' }),
      ],
      total: 2,
    })
    renderWithProviders(<DiscoveryTriage />)
    expect(screen.getByTestId('link-suggestion-row-1')).toBeInTheDocument()
    expect(screen.getByTestId('link-suggestion-row-2')).toBeInTheDocument()
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })

  it('visually distinguishes the review tier: shows the Verify badge + caveat, never on a high row', () => {
    mockList({
      suggestions: [
        makeEntry({ id: 1, confidence: 'high' }),
        makeEntry({ id: 2, confidence: 'review' }),
      ],
      total: 2,
    })
    renderWithProviders(<DiscoveryTriage />)
    // review row carries the Verify badge + caveat
    expect(screen.getByTestId('link-suggestion-verify-badge-2')).toBeInTheDocument()
    expect(screen.getByTestId('link-suggestion-caveat-2')).toBeInTheDocument()
    // high row does NOT — it shows "High confidence" and no caveat
    expect(
      screen.queryByTestId('link-suggestion-verify-badge-1')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('link-suggestion-caveat-1')
    ).not.toBeInTheDocument()
    expect(screen.getByText('High confidence')).toBeInTheDocument()
  })

  it('Accept calls the mutation with verdict "accept" for the row', () => {
    const mutate = stubReviewMutation('success')
    mockList({ suggestions: [makeEntry({ id: 7 })], total: 1 })
    renderWithProviders(<DiscoveryTriage />)

    fireEvent.click(screen.getByTestId('link-suggestion-accept-7'))

    expect(mutate).toHaveBeenCalledWith(
      { suggestionId: 7, verdict: 'accept' },
      expect.any(Object)
    )
  })

  it('Reject calls the mutation with verdict "reject" for the row', () => {
    const mutate = stubReviewMutation('success')
    mockList({ suggestions: [makeEntry({ id: 8 })], total: 1 })
    renderWithProviders(<DiscoveryTriage />)

    fireEvent.click(screen.getByTestId('link-suggestion-reject-8'))

    expect(mutate).toHaveBeenCalledWith(
      { suggestionId: 8, verdict: 'reject' },
      expect.any(Object)
    )
  })

  it('Spotify accept success says the embed renders now (immediate)', async () => {
    stubReviewMutation('success')
    mockList({
      suggestions: [makeEntry({ id: 9, platform: 'spotify', artist_name: 'Imma' })],
      total: 1,
    })
    renderWithProviders(<DiscoveryTriage />)

    fireEvent.click(screen.getByTestId('link-suggestion-accept-9'))

    await waitFor(() =>
      expect(
        screen.getByTestId('link-suggestion-success-banner')
      ).toBeInTheDocument()
    )
    expect(screen.getByText(/embed renders on the artist page now/i)).toBeInTheDocument()
  })

  it('Bandcamp accept success is HONEST: fills shortly via the background resolver, not instantly', async () => {
    stubReviewMutation('success')
    mockList({
      suggestions: [makeEntry({ id: 10, platform: 'bandcamp', artist_name: 'Doom' })],
      total: 1,
    })
    renderWithProviders(<DiscoveryTriage />)

    fireEvent.click(screen.getByTestId('link-suggestion-accept-10'))

    await waitFor(() =>
      expect(
        screen.getByTestId('link-suggestion-success-banner')
      ).toBeInTheDocument()
    )
    // Must NOT claim an instantly-live embed; must mention the async resolver.
    const banner = screen.getByTestId('link-suggestion-success-banner')
    expect(banner.textContent).toMatch(/fills in shortly/i)
    expect(banner.textContent).toMatch(/background/i)
  })

  it('surfaces a 409 conflicting-verdict error inline (does not silently drop the row)', async () => {
    const conflict = Object.assign(new Error('conflict'), { status: 409 })
    stubReviewMutation({ error: conflict })
    mockList({ suggestions: [makeEntry({ id: 11 })], total: 1 })
    renderWithProviders(<DiscoveryTriage />)

    fireEvent.click(screen.getByTestId('link-suggestion-accept-11'))

    await waitFor(() =>
      expect(screen.getByTestId('link-suggestion-error-11')).toBeInTheDocument()
    )
    expect(
      screen.getByText(/already reviewed with a different verdict/i)
    ).toBeInTheDocument()
    // The row is still present — not dropped.
    expect(screen.getByTestId('link-suggestion-row-11')).toBeInTheDocument()
  })

  it('surfaces a 422 invalid-URL error inline', async () => {
    const invalid = Object.assign(new Error('bad url'), { status: 422 })
    stubReviewMutation({ error: invalid })
    mockList({ suggestions: [makeEntry({ id: 12 })], total: 1 })
    renderWithProviders(<DiscoveryTriage />)

    fireEvent.click(screen.getByTestId('link-suggestion-accept-12'))

    await waitFor(() =>
      expect(screen.getByTestId('link-suggestion-error-12')).toBeInTheDocument()
    )
    expect(screen.getByText(/failed validation/i)).toBeInTheDocument()
  })

  it('renders the load-error fallback when the list query errors', () => {
    mockList(null, { isError: true, error: new Error('boom') })
    renderWithProviders(<DiscoveryTriage />)
    expect(screen.getByTestId('link-suggestion-load-error')).toBeInTheDocument()
  })

  it('shows pagination controls only when total exceeds the page size', () => {
    mockList({ suggestions: [makeEntry()], total: 50 })
    renderWithProviders(<DiscoveryTriage />)
    expect(screen.getByTestId('link-suggestion-next')).toBeInTheDocument()
    expect(screen.getByTestId('link-suggestion-prev')).toBeInTheDocument()
  })
})
