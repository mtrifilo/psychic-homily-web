import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { ArtistBillComposition } from '../types'

// Above-threshold mock with two opens-with rows + one closes-with row.
const mockData: ArtistBillComposition = {
  artist: {
    id: 1,
    name: 'Gatecreeper',
    slug: 'gatecreeper',
    city: 'Phoenix',
    state: 'AZ',
    upcoming_show_count: 3,
  },
  stats: {
    total_shows: 12,
    headliner_count: 8,
    opener_count: 4,
  },
  opens_with: [
    {
      artist: { id: 2, name: 'Frozen Soul', slug: 'frozen-soul', upcoming_show_count: 0 },
      shared_count: 5,
      last_shared: '2026-03-01',
    },
    {
      artist: { id: 3, name: 'Undeath', slug: 'undeath', upcoming_show_count: 0 },
      shared_count: 2,
      last_shared: '2025-11-12',
    },
  ],
  closes_with: [
    {
      artist: { id: 4, name: 'Cannibal Corpse', slug: 'cannibal-corpse', upcoming_show_count: 0 },
      shared_count: 3,
      last_shared: '2025-08-04',
    },
  ],
  graph: {
    center: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', upcoming_show_count: 3 },
    nodes: [
      { id: 2, name: 'Frozen Soul', slug: 'frozen-soul', upcoming_show_count: 0 },
      { id: 3, name: 'Undeath', slug: 'undeath', upcoming_show_count: 0 },
      { id: 4, name: 'Cannibal Corpse', slug: 'cannibal-corpse', upcoming_show_count: 0 },
    ],
    links: [
      { source_id: 1, target_id: 2, type: 'shared_bills', score: 0.5, votes_up: 0, votes_down: 0 },
      { source_id: 1, target_id: 3, type: 'shared_bills', score: 0.2, votes_up: 0, votes_down: 0 },
      { source_id: 1, target_id: 4, type: 'shared_bills', score: 0.3, votes_up: 0, votes_down: 0 },
    ],
  },
  below_threshold: false,
  time_filter_months: 0,
}

vi.mock('../hooks/useArtistBillComposition', () => ({
  useArtistBillComposition: vi.fn(() => ({
    data: mockData,
    isLoading: false,
    error: null,
  })),
}))

// Canvas-based ArtistGraph can't render in jsdom — replace with a stub.
vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: (props: { labelTiers?: unknown }) => (
    <div
      data-testid="bill-graph"
      data-has-label-tiers={String(props.labelTiers !== undefined)}
    >
      Bill Graph
    </div>
  ),
}))

import { BillComposition } from './BillComposition'

// Shared immediate ResizeObserver shim (PSY-1305) — drive container width so
// the >= 640px graph gate can be exercised.

describe('BillComposition', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    ro = installImmediateResizeObserver()
  })

  afterEach(() => {
    ro.restore()
  })

  it('renders the section header and stats', () => {
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.getByText('Bill composition')).toBeInTheDocument()
    expect(screen.getByText(/12 shows/)).toBeInTheDocument()
    expect(screen.getByText(/8 as headliner/)).toBeInTheDocument()
    expect(screen.getByText(/4 as opener/)).toBeInTheDocument()
  })

  it('renders opens-with rows in order with link + count', () => {
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.getByText('Opens with')).toBeInTheDocument()
    const frozenSoulLink = screen.getByText('Frozen Soul').closest('a')
    expect(frozenSoulLink).toHaveAttribute('href', '/artists/frozen-soul')
    expect(screen.getByText(/5 shows · last: 2026-03-01/)).toBeInTheDocument()
    expect(screen.getByText(/2 shows · last: 2025-11-12/)).toBeInTheDocument()
  })

  it('renders closes-with rows', () => {
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.getByText('Closes with')).toBeInTheDocument()
    expect(screen.getByText('Cannibal Corpse')).toBeInTheDocument()
    expect(screen.getByText(/3 shows · last: 2025-08-04/)).toBeInTheDocument()
  })

  it('shows the Explore graph button when graph has 3+ nodes at desktop width', () => {
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.getByText('Explore graph')).toBeInTheDocument()
  })

  it('hides the Explore graph button below the 640px breakpoint', () => {
    ro.setWidth(500)
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.queryByText('Explore graph')).not.toBeInTheDocument()
  })

  it('reveals the bill graph after clicking Explore graph', async () => {
    const user = userEvent.setup()
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.queryByTestId('bill-graph')).not.toBeInTheDocument()
    await user.click(screen.getByText('Explore graph'))
    const graph = screen.getByTestId('bill-graph')
    expect(graph).toBeInTheDocument()
    // Negative pin: bill composition is NOT a spec-classified tier surface —
    // it must keep the legacy flat label clamp, so no ladder is passed.
    expect(graph).toHaveAttribute('data-has-label-tiers', 'false')
  })

  it('switches the time filter and re-calls the hook with months=12', async () => {
    const hooks = await import('../hooks/useArtistBillComposition')
    const useHook = vi.mocked(hooks.useArtistBillComposition)
    useHook.mockClear()
    const user = userEvent.setup()
    renderWithProviders(<BillComposition artistId={1} />)
    expect(useHook).toHaveBeenCalledWith(expect.objectContaining({ months: 0 }))
    await user.click(screen.getByText('Last 12 months'))
    expect(useHook).toHaveBeenCalledWith(expect.objectContaining({ months: 12 }))
  })

  it('renders nothing when below_threshold is true', async () => {
    const hooks = await import('../hooks/useArtistBillComposition')
    vi.mocked(hooks.useArtistBillComposition).mockReturnValue({
      data: { ...mockData, below_threshold: true, opens_with: [], closes_with: [] },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(<BillComposition artistId={1} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders a height-reserving skeleton (not null) while loading (PSY-1446)', async () => {
    const hooks = await import('../hooks/useArtistBillComposition')
    vi.mocked(hooks.useArtistBillComposition).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(<BillComposition artistId={1} />)
    // Headerless pulse box: reserves space without a labeled section that
    // would vanish for below-threshold artists.
    expect(container.querySelector('.animate-pulse')).not.toBeNull()
    expect(screen.queryByText('Bill composition')).not.toBeInTheDocument()
  })

  it('shows empty-state copy when one bucket has rows and the other does not', async () => {
    const hooks = await import('../hooks/useArtistBillComposition')
    vi.mocked(hooks.useArtistBillComposition).mockReturnValue({
      data: { ...mockData, closes_with: [] },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.getByText("Hasn't opened for anyone yet.")).toBeInTheDocument()
  })
})

describe('BillComposition — defaultCollapsed (PSY-644)', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    ro = installImmediateResizeObserver()
  })

  afterEach(() => {
    ro.restore()
  })

  it('renders only the header with a [Show] toggle when defaultCollapsed', () => {
    renderWithProviders(<BillComposition artistId={1} defaultCollapsed />)
    expect(screen.getByRole('heading', { name: 'Bill composition' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show' })).toBeInTheDocument()
    // Stats line + opens-with rows hidden while collapsed.
    expect(screen.queryByText(/as headliner/)).not.toBeInTheDocument()
    expect(screen.queryByText('Frozen Soul')).not.toBeInTheDocument()
  })

  it('reveals the body when the [Show] toggle is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<BillComposition artistId={1} defaultCollapsed />)
    await user.click(screen.getByRole('button', { name: 'Show' }))
    expect(screen.getByText(/as headliner/)).toBeInTheDocument()
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    // Toggle label flips to [Hide].
    expect(screen.getByRole('button', { name: 'Hide' })).toBeInTheDocument()
  })

  it('without defaultCollapsed, renders body eagerly (pre-PSY-644 behavior)', () => {
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.getByText(/as headliner/)).toBeInTheDocument()
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Show' })).not.toBeInTheDocument()
  })
})
