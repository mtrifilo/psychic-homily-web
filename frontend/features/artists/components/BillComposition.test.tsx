import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
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
  ArtistGraphVisualization: () => <div data-testid="bill-graph">Bill Graph</div>,
}))

import { BillComposition } from './BillComposition'

// Same pattern as RelatedArtists.test.tsx — drive container width through a
// synchronous ResizeObserver so the >= 640px graph gate can be exercised.
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

describe('BillComposition', () => {
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
    setMockContainerWidth(500)
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.queryByText('Explore graph')).not.toBeInTheDocument()
  })

  it('reveals the bill graph after clicking Explore graph', async () => {
    const user = userEvent.setup()
    renderWithProviders(<BillComposition artistId={1} />)
    expect(screen.queryByTestId('bill-graph')).not.toBeInTheDocument()
    await user.click(screen.getByText('Explore graph'))
    expect(screen.getByTestId('bill-graph')).toBeInTheDocument()
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

  it('renders nothing while loading', async () => {
    const hooks = await import('../hooks/useArtistBillComposition')
    vi.mocked(hooks.useArtistBillComposition).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(<BillComposition artistId={1} />)
    expect(container.firstChild).toBeNull()
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
