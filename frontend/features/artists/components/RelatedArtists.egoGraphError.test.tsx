import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { ArtistGraph } from '../types'

// PSY-1371: a failed react-force-graph-2d chunk fetch inside the ego-graph dialog
// throws (App Router). Verify GraphSectionErrorBoundary contains it to the dialog
// (Sentry-reported, recoverable card) instead of crashing the whole artist page.

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

const mockGraphData: ArtistGraph = {
  center: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', city: 'Phoenix', state: 'AZ', upcoming_show_count: 3 },
  nodes: [{ id: 2, name: 'Frozen Soul', slug: 'frozen-soul', city: 'Fort Worth', state: 'TX', upcoming_show_count: 1 }],
  links: [{ source_id: 1, target_id: 2, type: 'similar', score: 0.85, votes_up: 8, votes_down: 2 }],
  user_votes: {},
}

vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: () => ({ data: mockGraphData, isLoading: false, error: null }),
  useFetchArtistGraph: () => vi.fn(),
  useArtistRelationshipVote: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useCreateArtistRelationship: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => ({ user: { id: 1, is_admin: false }, isAuthenticated: true }),
}))

vi.mock('next/navigation', () => ({
  useRouter: vi.fn(() => ({ push: vi.fn() })),
  usePathname: vi.fn(() => '/artists/gatecreeper'),
  useSearchParams: vi.fn(() => new URLSearchParams()),
}))

// The canvas viz stands in for the dynamic chunk: throwing here is exactly what a
// failed react-force-graph-2d chunk fetch does at render.
vi.mock('./ArtistGraph', async () => {
  const { createContext } = await import('react')
  return {
    ArtistGraphVisualization: () => {
      throw new Error('chunk fetch failed')
    },
    ConnectionPanelDismissContext: createContext(null),
    // RelatedArtists also imports this for the Dialog's onEscapeKeyDown; stubbed
    // so the mock is a complete stand-in (not exercised here — no Esc keypress).
    dismissConnectionPanelOnEscape: vi.fn(),
  }
})

import { ArtistGraphDialog } from './RelatedArtists'

describe('ArtistGraphDialog — ego-graph error boundary (PSY-1371)', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    captureException.mockReset()
    ro = installImmediateResizeObserver()
  })
  afterEach(() => {
    ro.restore()
  })

  it('contains a graph chunk failure to the dialog and reports it to Sentry', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open
        onOpenChange={vi.fn()}
      />,
    )

    // Contained, not an uncaught page crash: the dialog + its title survive
    // (the Dialog's own close X/Esc remains the recovery), and the recoverable
    // card shows instead of the graph.
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText(/Similar artists · Gatecreeper/)).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent(/couldn.t load/i)

    // Reported to Sentry under the ego-graph tag.
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'artist-ego-graph' } }),
    )

    spy.mockRestore()
  })
})
