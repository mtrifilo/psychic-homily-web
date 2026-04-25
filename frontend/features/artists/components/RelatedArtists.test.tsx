import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// Mock the hooks
const mockGraphData: ArtistGraph = {
  center: {
    id: 1,
    name: 'Gatecreeper',
    slug: 'gatecreeper',
    city: 'Phoenix',
    state: 'AZ',
    upcoming_show_count: 3,
  },
  nodes: [
    {
      id: 2,
      name: 'Frozen Soul',
      slug: 'frozen-soul',
      city: 'Fort Worth',
      state: 'TX',
      upcoming_show_count: 1,
    },
    {
      id: 3,
      name: 'Undeath',
      slug: 'undeath',
      city: 'Rochester',
      state: 'NY',
      upcoming_show_count: 2,
    },
    {
      id: 4,
      name: 'Creeping Death',
      slug: 'creeping-death',
      city: 'Dallas',
      state: 'TX',
      upcoming_show_count: 0,
    },
  ],
  links: [
    {
      source_id: 1,
      target_id: 2,
      type: 'similar',
      score: 0.85,
      votes_up: 8,
      votes_down: 2,
    },
    {
      source_id: 1,
      target_id: 3,
      type: 'similar',
      score: 0.68,
      votes_up: 5,
      votes_down: 1,
    },
    {
      source_id: 1,
      target_id: 4,
      type: 'shared_bills',
      score: 0.6,
      votes_up: 0,
      votes_down: 0,
      detail: { shared_count: 4, last_shared: '2026-03-01' },
    },
  ],
  user_votes: {
    '1-2-similar': 'up',
  },
}

vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: vi.fn(() => ({
    data: mockGraphData,
    isLoading: false,
    error: null,
  })),
  useArtistRelationshipVote: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
  useCreateArtistRelationship: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
}))

vi.mock('@/features/auth', () => ({
  useIsAuthenticated: vi.fn(() => ({
    user: { id: 1, is_admin: false },
    isAuthenticated: true,
  })),
}))

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useRouter: vi.fn(() => ({
    push: vi.fn(),
  })),
}))

// Mock the ArtistGraph visualization (canvas-based, can't render in jsdom)
vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: () => <div data-testid="artist-graph">Graph Visualization</div>,
}))

import { RelatedArtists } from './RelatedArtists'

// PSY-511: RelatedArtists now defers the View Map button + graph until
// ResizeObserver reports a real container width (>= 640px). The shared
// ResizeObserver mock in test/setup.ts never fires its callback, so we
// override it locally with one that synchronously reports a configurable
// width. Each test sets the width via setMockContainerWidth() before
// rendering; the default (1024) is wide enough that the View Map button
// renders, matching desktop behaviour.
let mockContainerWidth = 1024

function setMockContainerWidth(width: number) {
  mockContainerWidth = width
}

class ImmediateResizeObserver {
  private callback: ResizeObserverCallback
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }
  observe(target: Element): void {
    // Fire synchronously so the component's useEffect picks up the
    // measured width on first commit, rather than waiting on a real
    // browser layout pass.
    this.callback(
      [
        {
          target,
          contentRect: { width: mockContainerWidth } as DOMRectReadOnly,
        } as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver
    )
  }
  unobserve(): void {}
  disconnect(): void {}
}

describe('RelatedArtists', () => {
  // The shared ResizeObserver mock in test/setup.ts is defined with
  // `writable: true` (and no `configurable`), so re-assignment works
  // even though Object.defineProperty does not. We swap it back to the
  // original after the suite so neighbouring tests aren't affected.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const originalResizeObserver = (window as any).ResizeObserver

  beforeEach(() => {
    setMockContainerWidth(1024)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = ImmediateResizeObserver
  })

  afterEach(() => {
    setMockContainerWidth(1024)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = originalResizeObserver
  })

  it('renders the section header', () => {
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    expect(screen.getByText('Related Artists')).toBeInTheDocument()
  })

  it('renders related artist names as links', () => {
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    expect(screen.getByText('Undeath')).toBeInTheDocument()
    expect(screen.getByText('Creeping Death')).toBeInTheDocument()
  })

  it('shows relationship type badges', () => {
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    // "Similar" badges for Frozen Soul and Undeath
    const similarBadges = screen.getAllByText('Similar')
    expect(similarBadges.length).toBeGreaterThanOrEqual(2)

    // "Shared Bills" badge for Creeping Death
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
  })

  it('shows the View Map button when 3+ nodes exist', () => {
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    expect(screen.getByText('View Map')).toBeInTheDocument()
  })

  it('shows suggest similar artist button for authenticated users', () => {
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    expect(screen.getByText('Suggest similar artist')).toBeInTheDocument()
  })

  it('shows vote buttons for similar relationships when authenticated', () => {
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    // Should have upvote and downvote buttons for similarity votes
    const upvoteButtons = screen.getAllByTitle('Upvote similarity')
    const downvoteButtons = screen.getAllByTitle('Downvote similarity')
    expect(upvoteButtons.length).toBeGreaterThanOrEqual(2)
    expect(downvoteButtons.length).toBeGreaterThanOrEqual(2)
  })

  it('shows empty state with suggest button when no relationships exist', async () => {
    const hooks = await import('../hooks/useArtistGraph')
    // mockReturnValue (not Once): the synchronous ResizeObserver causes
    // a re-render after the initial commit, so the hook is called more
    // than once per test — Once would let the second render fall back
    // to the populated default mock and break the assertion.
    vi.mocked(hooks.useArtistGraph).mockReturnValue({
      data: {
        center: { id: 1, name: 'Lonely', slug: 'lonely', upcoming_show_count: 0 },
        nodes: [],
        links: [],
      },
      isLoading: false,
      error: null,
    } as any) // eslint-disable-line @typescript-eslint/no-explicit-any

    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="lonely" />
    )
    // Should show the section header and empty state message
    expect(screen.getByText('Related Artists')).toBeInTheDocument()
    expect(screen.getByText('No similar artists yet. Be the first to suggest one!')).toBeInTheDocument()
    // Should show the suggest button for authenticated users
    expect(screen.getByText('Suggest similar artist')).toBeInTheDocument()

    // Restore the default for subsequent tests in this suite.
    vi.mocked(hooks.useArtistGraph).mockReturnValue({
      data: mockGraphData,
      isLoading: false,
      error: null,
    } as any) // eslint-disable-line @typescript-eslint/no-explicit-any
  })

  it('hides section while loading', async () => {
    const hooks = await import('../hooks/useArtistGraph')
    vi.mocked(hooks.useArtistGraph).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as any) // eslint-disable-line @typescript-eslint/no-explicit-any

    const { container } = renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="loading-artist" />
    )
    expect(container.children.length).toBe(0)

    // Restore the default for subsequent tests in this suite.
    vi.mocked(hooks.useArtistGraph).mockReturnValue({
      data: mockGraphData,
      isLoading: false,
      error: null,
    } as any) // eslint-disable-line @typescript-eslint/no-explicit-any
  })

  // PSY-511: below 640px (Tailwind `sm`) the graph is unusable on a
  // phone. Hide the View Map button entirely — the list view is the
  // only surface, no "best viewed on desktop" nag.
  it('hides the View Map button on narrow viewports (< 640px)', () => {
    setMockContainerWidth(375)
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    // List view (artists by name) still renders.
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    // View Map button is gated off.
    expect(screen.queryByText('View Map')).not.toBeInTheDocument()
    expect(screen.queryByText('Hide Map')).not.toBeInTheDocument()
  })

  it('shows the View Map button at exactly the 640px breakpoint', () => {
    setMockContainerWidth(640)
    renderWithProviders(
      <RelatedArtists artistId={1} artistSlug="gatecreeper" />
    )
    expect(screen.getByText('View Map')).toBeInTheDocument()
  })
})
