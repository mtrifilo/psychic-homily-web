import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { ArtistBillComposition } from '../types'

// PSY-1371: BillComposition's inline graph loads the SAME module-scope
// react-force-graph-2d chunk as the ego dialog. Verify a failed chunk fetch here
// is contained by GraphSectionErrorBoundary (reported, recoverable card) instead
// of crashing the whole artist page.

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

const mockData: ArtistBillComposition = {
  artist: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', city: 'Phoenix', state: 'AZ', upcoming_show_count: 3 },
  stats: { total_shows: 12, headliner_count: 8, opener_count: 4 },
  opens_with: [
    { artist: { id: 2, name: 'Frozen Soul', slug: 'frozen-soul', upcoming_show_count: 0 }, shared_count: 5, last_shared: '2026-03-01' },
    { artist: { id: 3, name: 'Undeath', slug: 'undeath', upcoming_show_count: 0 }, shared_count: 2, last_shared: '2025-11-12' },
  ],
  closes_with: [
    { artist: { id: 4, name: 'Cannibal Corpse', slug: 'cannibal-corpse', upcoming_show_count: 0 }, shared_count: 3, last_shared: '2025-08-04' },
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
  useArtistBillComposition: vi.fn(() => ({ data: mockData, isLoading: false, error: null })),
}))

// Throwing viz stands in for the failed react-force-graph-2d chunk fetch.
vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: () => {
    throw new Error('chunk fetch failed')
  },
}))

import { BillComposition } from './BillComposition'

describe('BillComposition — inline graph error boundary (PSY-1371)', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    captureException.mockReset()
    ro = installImmediateResizeObserver()
  })
  afterEach(() => {
    ro.restore()
  })

  it('contains a graph chunk failure instead of crashing the page, and reports it', async () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    renderWithProviders(<BillComposition artistId={1} />)

    // Expand the inline graph — this is where the throwing viz mounts.
    await userEvent.click(screen.getByText('Explore graph'))

    // Contained: the recoverable card shows, and the rest of BillComposition
    // (the tables) is unaffected — no uncaught throw to app/error.tsx.
    expect(screen.getByRole('alert')).toHaveTextContent(/couldn.t load/i)
    expect(screen.getByText('Opens with')).toBeInTheDocument()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'artist-bill-composition' } }),
    )

    spy.mockRestore()
  })
})
