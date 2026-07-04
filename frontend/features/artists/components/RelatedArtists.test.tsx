import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
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

type MockUseArtistGraphValue = {
  data: ArtistGraph | undefined
  isLoading: boolean
  error: Error | null
}
const mockUseArtistGraph = vi.fn<(_opts?: unknown) => MockUseArtistGraphValue>(
  () => ({
    data: mockGraphData,
    isLoading: false,
    error: null,
  })
)

vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: (opts: unknown) => mockUseArtistGraph(opts),
  // PSY-1259: the imperative expand fetcher. Returns a stable no-op fetcher — these tests
  // exercise the dialog/filter/festival flows, not expand-on-demand (covered by the
  // mergeEgoGraphs unit tests + browser verification).
  useFetchArtistGraph: () => vi.fn(),
  useArtistRelationshipVote: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useCreateArtistRelationship: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

type MockUseIsAuthenticatedValue = {
  user: { id: number; is_admin: boolean } | null
  isAuthenticated: boolean
}
const mockUseIsAuthenticated = vi.fn<() => MockUseIsAuthenticatedValue>(() => ({
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
vi.mock('./ArtistGraph', async () => {
  const { createContext } = await import('react')
  return {
    ArtistGraphVisualization: () => <div data-testid="artist-graph">Graph Visualization</div>,
    // PSY-1351: ArtistGraphDialog wraps the graph in this context's Provider.
    ConnectionPanelDismissContext: createContext(null),
  }
})

import { ArtistSimilarSidebar, ArtistGraphDialog } from './RelatedArtists'

// ResizeObserver mock — the Dialog measures its content via ResizeObserver.
// Shared immediate shim (PSY-1305).

describe('ArtistSimilarSidebar', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    mockUseArtistGraph.mockReturnValue({ data: mockGraphData, isLoading: false, error: null })
    mockUseIsAuthenticated.mockReturnValue({
      user: { id: 1, is_admin: false },
      isAuthenticated: true,
    })
    ro = installImmediateResizeObserver()
  })

  afterEach(() => {
    ro.restore()
  })

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

  // PSY-954: the sidebar fetches the backend default (no `types`), which now
  // returns STORED types only — festival_cobill is excluded, so the sidebar
  // can never surface a bogus "similar via 1 shared festival" entry.
  it('fetches the default graph with no types filter (stored types only)', () => {
    mockUseArtistGraph.mockClear()
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    const opts = mockUseArtistGraph.mock.calls[0]?.[0] as { types?: unknown } | undefined
    expect(opts?.types).toBeUndefined()
  })

  // PSY-1280: accessible DOI sort. Default = max-edge-score order; a Discovery mode re-orders the
  // list by the canvas's Degree-of-Interest ranking (motion-free DOM reflow), announced via aria-live.
  // mockGraphData scores: Frozen Soul similar 0.85, Undeath similar 0.68, Creeping Death shared_bills 0.6.
  // Document order of the unique artist-name nodes:
  const artistOrder = (): string[] => {
    const names = ['Frozen Soul', 'Undeath', 'Creeping Death']
    return names
      .map(n => ({ n, el: screen.getByText(n) }))
      .sort((a, b) =>
        a.el.compareDocumentPosition(b.el) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1,
      )
      .map(x => x.n)
  }

  it('defaults to the max-edge-score order with the sort control set to Most relevant', () => {
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    // Score order: 0.85 > 0.68 > 0.6.
    expect(artistOrder()).toEqual(['Frozen Soul', 'Undeath', 'Creeping Death'])
    expect((screen.getByLabelText('Sort') as HTMLSelectElement).value).toBe('relevant')
  })

  it('re-orders by DOI and announces it when a Discovery mode is chosen', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    await user.selectOptions(screen.getByLabelText('Sort'), 'niche')

    // DOI normalizes relevance PER edge type, so Creeping Death's shared_bills (the strongest of
    // its type → 1.0) beats Undeath's weaker similar tie (0.68/0.85 = 0.8); that per-type gap is
    // THEN min-max-normalized across the scored set (graphDoi.ts), widening 1.0-vs-0.8 to the full
    // relevance weight — so Creeping Death moves above Undeath despite a lower raw score (the whole
    // point of surfacing DOI). (Frozen Soul and Creeping Death tie on DOI here; the score tiebreak
    // keeps Frozen Soul top.)
    expect(artistOrder()).toEqual(['Frozen Soul', 'Creeping Death', 'Undeath'])
    // aria-live announcement of the re-rank.
    expect(
      screen.getByText('Similar artists sorted by discovery, niche-first.')
    ).toBeInTheDocument()
    // AC: the SAME artists are shown across modes — DOI only re-orders, never adds/removes.
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    expect(screen.getByText('Undeath')).toBeInTheDocument()
    expect(screen.getByText('Creeping Death')).toBeInTheDocument()
  })

  it('restores the default order when switched back to Most relevant', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    await user.selectOptions(screen.getByLabelText('Sort'), 'popular')
    expect(artistOrder()).toEqual(['Frozen Soul', 'Creeping Death', 'Undeath'])
    await user.selectOptions(screen.getByLabelText('Sort'), 'relevant')
    expect(artistOrder()).toEqual(['Frozen Soul', 'Undeath', 'Creeping Death'])
    expect(
      screen.getByText('Similar artists sorted by most relevant.')
    ).toBeInTheDocument()
  })

  it('hides the sort control when there is at most one related artist', () => {
    mockUseArtistGraph.mockReturnValue({
      data: {
        ...mockGraphData,
        nodes: [mockGraphData.nodes[0]],
        links: [mockGraphData.links[0]],
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    expect(screen.queryByLabelText('Sort')).not.toBeInTheDocument()
  })

  // The star fixture above (every neighbor degree 1) CANNOT distinguish popular from niche: the
  // bias only moves DOI's importance term (in-subgraph degree), which degenerate-normalizes to a
  // constant when all degrees are equal — so both modes produce the same order, and a swapped/
  // broken SORT_MODE_BIAS would still pass. This fixture varies degree: "High Degree" (id 2) has
  // cross-edges to two other neighbors (degree 3); "Low Degree" (id 3) has only its center edge
  // (degree 1) with the SAME center-tie strength (0.8) → they differ ONLY in importance. The
  // cross-edges are radio (not center edges), so the sidebar's score-sort never sees them — only
  // DOI does, via the full merged graph. So popular (importance +) ranks High Degree up; niche
  // (importance −) ranks it down. They must FLIP — which is the whole point of the two modes.
  const hubGraphData: ArtistGraph = {
    center: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', upcoming_show_count: 0 },
    nodes: [
      { id: 2, name: 'High Degree', slug: 'high-degree', upcoming_show_count: 0 },
      { id: 3, name: 'Low Degree', slug: 'low-degree', upcoming_show_count: 0 },
      { id: 4, name: 'Cross One', slug: 'cross-one', upcoming_show_count: 0 },
      { id: 5, name: 'Cross Two', slug: 'cross-two', upcoming_show_count: 0 },
    ],
    links: [
      { source_id: 1, target_id: 2, type: 'similar', score: 0.8, votes_up: 0, votes_down: 0 },
      { source_id: 1, target_id: 3, type: 'similar', score: 0.8, votes_up: 0, votes_down: 0 },
      { source_id: 1, target_id: 4, type: 'similar', score: 0.5, votes_up: 0, votes_down: 0 },
      { source_id: 1, target_id: 5, type: 'similar', score: 0.5, votes_up: 0, votes_down: 0 },
      // Cross-edges give "High Degree" (id 2) its extra in-subgraph degree.
      { source_id: 2, target_id: 4, type: 'radio_cooccurrence', score: 0.5, votes_up: 0, votes_down: 0 },
      { source_id: 2, target_id: 5, type: 'radio_cooccurrence', score: 0.5, votes_up: 0, votes_down: 0 },
    ],
    user_votes: {},
  }

  it('flips popular-first vs niche-first by in-subgraph degree (proves the bias mapping)', async () => {
    const user = userEvent.setup()
    mockUseArtistGraph.mockReturnValue({ data: hubGraphData, isLoading: false, error: null })
    renderWithProviders(
      <ArtistSimilarSidebar artistId={1} artistSlug="gatecreeper" onOpenGraph={() => {}} />
    )
    const isBefore = (a: string, b: string) =>
      Boolean(
        screen.getByText(a).compareDocumentPosition(screen.getByText(b)) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      )

    await user.selectOptions(screen.getByLabelText('Sort'), 'popular')
    expect(isBefore('High Degree', 'Low Degree')).toBe(true) // popular favors the high-degree hub

    await user.selectOptions(screen.getByLabelText('Sort'), 'niche')
    expect(isBefore('Low Degree', 'High Degree')).toBe(true) // niche favors the low-degree leaf
  })
})

describe('ArtistGraphDialog', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    mockUseArtistGraph.mockReturnValue({ data: mockGraphData, isLoading: false, error: null })
    ro = installImmediateResizeObserver()
  })

  afterEach(() => {
    ro.restore()
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

// PSY-954: festival co-lineup is opt-in. The default graph fetch must NOT
// request festival_cobill; the festival toggle always renders and turning it
// on lazy-fetches with festival_cobill in `types`.
describe('ArtistGraphDialog — festival_cobill opt-in (PSY-954)', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    mockUseArtistGraph.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: mockGraphData, isLoading: false, error: null })
    ro = installImmediateResizeObserver()
  })

  afterEach(() => {
    ro.restore()
  })

  // Helper: pull the `types` arg out of the most recent useArtistGraph call
  // that came from the RecenteringGraph fetch (the one with a positive id).
  const lastFetchTypes = (): unknown => {
    const calls = mockUseArtistGraph.mock.calls
    const last = calls[calls.length - 1]?.[0] as { types?: unknown } | undefined
    return last?.types
  }

  it('fetches the default graph (no festival_cobill in types) on open', () => {
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open={true}
        onOpenChange={() => {}}
      />
    )
    // Default: festival toggle OFF → fetch types is undefined (stored types only).
    expect(lastFetchTypes()).toBeUndefined()
  })

  it('always renders the Festival co-lineup toggle even when absent from the payload', () => {
    // mockGraphData has only `similar` + `shared_bills` links — no festival.
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
      screen.getByRole('button', { name: /Festival co-lineup/ })
    ).toBeInTheDocument()
  })

  it('does not show the Festival co-lineup toggle as active by default', () => {
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open={true}
        onOpenChange={() => {}}
      />
    )
    // Inactive toggles render at opacity-60 (PSY-1290 bumped from 40 for light-mode legibility);
    // active at opacity-100. Asserting the inactive value confirms the toggle is not active by default.
    const toggle = screen.getByRole('button', { name: /Festival co-lineup/ })
    expect(toggle.className).toContain('opacity-60')
    expect(toggle.className).not.toContain('opacity-100')
  })

  it('lazy-fetches with festival_cobill in types when the toggle is turned on', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ArtistGraphDialog
        artistId={1}
        artistSlug="gatecreeper"
        artistName="Gatecreeper"
        open={true}
        onOpenChange={() => {}}
      />
    )
    await user.click(screen.getByRole('button', { name: /Festival co-lineup/ }))
    // After opt-in the fetch carries festival_cobill (alongside stored types so
    // the backend keeps returning stored relationships).
    const types = lastFetchTypes()
    expect(Array.isArray(types)).toBe(true)
    expect(types as string[]).toContain('festival_cobill')
  })
})
