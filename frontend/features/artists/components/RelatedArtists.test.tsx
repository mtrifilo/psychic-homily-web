import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// Default fixture — Gatecreeper with three related artists. Tests override
// via `mockUseArtistGraph.mockReturnValue(...)` per case.
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
    { id: 2, name: 'Frozen Soul', slug: 'frozen-soul', city: 'Fort Worth', state: 'TX', upcoming_show_count: 1 },
    { id: 3, name: 'Undeath', slug: 'undeath', city: 'Rochester', state: 'NY', upcoming_show_count: 2 },
    { id: 4, name: 'Creeping Death', slug: 'creeping-death', city: 'Dallas', state: 'TX', upcoming_show_count: 0 },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'similar', score: 0.85, votes_up: 8, votes_down: 2 },
    { source_id: 1, target_id: 3, type: 'similar', score: 0.68, votes_up: 5, votes_down: 1 },
    { source_id: 1, target_id: 4, type: 'shared_bills', score: 0.6, votes_up: 0, votes_down: 0, detail: { shared_count: 4, last_shared: '2026-03-01' } },
  ],
  user_votes: { '1-2-similar': 'up' },
}

const mockUseArtistGraph = vi.fn((_opts?: unknown) => ({
  data: mockGraphData,
  isLoading: false,
  error: null,
}))

vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: (opts: unknown) => mockUseArtistGraph(opts),
  useArtistRelationshipVote: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useCreateArtistRelationship: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

const mockUseIsAuthenticated = vi.fn(() => ({
  user: { id: 1, is_admin: false },
  isAuthenticated: true,
}))

vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

// `RecenteringGraph` (rendered inside the Dialog) calls usePathname +
// useSearchParams; vitest throws "No <hook> export is defined" without them.
vi.mock('next/navigation', () => ({
  useRouter: vi.fn(() => ({ push: vi.fn() })),
  usePathname: vi.fn(() => '/artists/gatecreeper'),
  useSearchParams: vi.fn(() => new URLSearchParams()),
}))

// Mock the canvas-based ArtistGraph viz.
vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: () => <div data-testid="artist-graph">Graph Visualization</div>,
}))

import { ArtistSimilarSidebar, ArtistGraphDialog } from './RelatedArtists'

// ResizeObserver mock — the Dialog measures its content via ResizeObserver.
class ImmediateResizeObserver {
  private callback: ResizeObserverCallback
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }
  observe(target: Element): void {
    this.callback(
      [{ target, contentRect: { width: 1024 } as DOMRectReadOnly } as ResizeObserverEntry],
      this as unknown as ResizeObserver
    )
  }
  unobserve(): void {}
  disconnect(): void {}
}

describe('ArtistSimilarSidebar', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const originalResizeObserver = (window as any).ResizeObserver

  beforeEach(() => {
    mockUseArtistGraph.mockReturnValue({ data: mockGraphData, isLoading: false, error: null })
    mockUseIsAuthenticated.mockReturnValue({
      user: { id: 1, is_admin: false },
      isAuthenticated: true,
    })
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = ImmediateResizeObserver
  })

  // eslint-disable-next-line vitest/no-disabled-tests
  it('returns null while loading', () => {
    mockUseArtistGraph.mockReturnValue({ data: undefined, isLoading: true, error: null })
    const { container } = renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('hides the section entirely for unauthenticated users when there are no relationships', () => {
    mockUseArtistGraph.mockReturnValue({
      data: { ...mockGraphData, nodes: [], links: [] },
      isLoading: false,
      error: null,
    })
    mockUseIsAuthenticated.mockReturnValue({ user: null, isAuthenticated: false })
    const { container } = renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders the Similar artists section header', () => {
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.getByText('Similar artists')).toBeInTheDocument()
  })

  it('renders related artist names from the graph', () => {
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    expect(screen.getByText('Undeath')).toBeInTheDocument()
    expect(screen.getByText('Creeping Death')).toBeInTheDocument()
  })

  it('renders an [Explore graph] link when relationships exist', () => {
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.getByRole('button', { name: 'Explore graph' })).toBeInTheDocument()
  })

  it('omits the [Explore graph] link when there are no relationships', () => {
    mockUseArtistGraph.mockReturnValue({
      data: { ...mockGraphData, nodes: [], links: [] },
      isLoading: false,
      error: null,
    })
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.queryByRole('button', { name: 'Explore graph' })).not.toBeInTheDocument()
  })

  it('calls onOpenGraph when [Explore graph] is clicked', async () => {
    const user = userEvent.setup()
    const onOpenGraph = vi.fn()
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={onOpenGraph} />
    )
    await user.click(screen.getByRole('button', { name: 'Explore graph' }))
    expect(onOpenGraph).toHaveBeenCalledOnce()
  })

  it('shows a [Suggest similar] affordance for authenticated users', () => {
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.getByRole('button', { name: 'Suggest similar' })).toBeInTheDocument()
  })

  it('does not show [Suggest similar] for unauthenticated users', () => {
    mockUseIsAuthenticated.mockReturnValue({ user: null, isAuthenticated: false })
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.queryByRole('button', { name: 'Suggest similar' })).not.toBeInTheDocument()
  })
})

describe('ArtistGraphDialog', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const originalResizeObserver = (window as any).ResizeObserver

  beforeEach(() => {
    mockUseArtistGraph.mockReturnValue({ data: mockGraphData, isLoading: false, error: null })
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = ImmediateResizeObserver
  })

  it('renders nothing when open=false (Radix lazy mount)', () => {
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open={false}
        onOpenChange={() => {}}
      />
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders the Dialog with the artist name in the title when open', () => {
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open={true}
        onOpenChange={() => {}}
      />
    )
    expect(
      screen.getByRole('dialog', { name: /Similar artists/ })
    ).toBeInTheDocument()
  })

  it('mounts the graph visualization inside the Dialog', () => {
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open={true}
        onOpenChange={() => {}}
      />
    )
    expect(screen.getByTestId('artist-graph')).toBeInTheDocument()
  })
})
