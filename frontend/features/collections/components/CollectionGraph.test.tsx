import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { CollectionGraphResponse } from '../types'

// Mock the collections hooks barrel before CollectionGraph imports it.
const mockData: CollectionGraphResponse = {
  collection: {
    slug: 'desert-doom',
    name: 'Desert Doom',
    artist_count: 2,
    edge_count: 1,
    entity_counts: { artist: 2, venue: 1 },
    node_total: 3,
    nodes_truncated: false,
  },
  nodes: [
    {
      id: 1,
      entity_type: 'artist',
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      is_isolate: false,
    },
    {
      id: 2,
      entity_type: 'artist',
      name: 'Spirit Adrift',
      slug: 'spirit-adrift',
      upcoming_show_count: 0,
      is_isolate: false,
    },
    {
      id: 3,
      entity_type: 'venue',
      name: 'Valley Bar',
      slug: 'valley-bar-phoenix-az',
      upcoming_show_count: 0,
      is_isolate: true,
    },
  ],
  links: [{ source_id: 1, target_id: 2, type: 'shared_bills', score: 0.5 }],
}

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('../hooks', () => ({
  useCollectionGraph: vi.fn(() => ({
    data: mockData,
    isLoading: false,
    isError: false,
  })),
}))

// jsdom can't render canvas; stub the shared graph view.
vi.mock('@/components/graph/ForceGraphView', () => ({
  ForceGraphView: () => (
    <div data-testid="collection-graph-canvas">Collection Graph Canvas</div>
  ),
}))

import { CollectionGraph } from './CollectionGraph'

describe('CollectionGraph (PSY-1446 states)', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(async () => {
    ro = installImmediateResizeObserver(1024)
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockImplementation(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      () => ({ data: mockData, isLoading: false, isError: false }) as any,
    )
  })

  afterEach(() => {
    ro.restore()
  })

  it('renders the canvas at desktop width', () => {
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.getByTestId('collection-graph-canvas')).toBeInTheDocument()
  })

  it('keeps the per-type breakdown + "every item" caption when not truncated (PSY-1476)', () => {
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.getByText(/2 artists · 1 venue/)).toBeInTheDocument()
    expect(screen.queryByText(/Top 3 of/)).not.toBeInTheDocument()
    expect(screen.getByText(/Showing every item in this collection/)).toBeInTheDocument()
  })

  it('replaces the breakdown + retitles the caption when truncated (PSY-1476)', async () => {
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: {
        ...mockData,
        // 3 nodes shown, but 312 items in the collection before the cap.
        collection: { ...mockData.collection, node_total: 312, nodes_truncated: true },
      },
      isLoading: false,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    // Header: cue replaces the per-type breakdown (would contradict the cap).
    expect(screen.getByText(/Top 3 of 312 items/)).toBeInTheDocument()
    expect(screen.queryByText(/2 artists/)).not.toBeInTheDocument()
    // Caption no longer claims "every item".
    expect(screen.getByText(/Showing the top 3 of 312 items in this collection/)).toBeInTheDocument()
    expect(screen.queryByText(/Showing every item/)).not.toBeInTheDocument()
  })

  it('reads "No items", never "Top 0 of N", when every node was dropped (PSY-1476)', async () => {
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: {
        // Deleted-entity payload the backend flags: 0 nodes, positive total.
        collection: {
          ...mockData.collection,
          artist_count: 0,
          edge_count: 0,
          entity_counts: {},
          node_total: 5,
          nodes_truncated: true,
        },
        nodes: [],
        links: [],
      },
      isLoading: false,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    // The guard degrades to the plain count — never "top 0 of 5".
    expect(screen.queryByText(/Top 0 of/)).not.toBeInTheDocument()
    // Empty-state body still renders (nodeCount === 0 branch).
    expect(screen.getByText(/No items yet/)).toBeInTheDocument()
  })

  it('renders a height-reserving skeleton (not bare text) while loading', async () => {
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />,
    )
    expect(container.querySelector('.animate-pulse')).not.toBeNull()
    expect(screen.queryByText(/Loading graph/)).not.toBeInTheDocument()
    expect(screen.queryByTestId('collection-graph-canvas')).not.toBeInTheDocument()
  })

  it('shows a visible error card when the graph fetch settles in error', async () => {
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Internal Server Error'),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.getByText('Collection graph')).toBeInTheDocument()
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(/couldn't load/i)
    expect(screen.queryByTestId('collection-graph-canvas')).not.toBeInTheDocument()
  })

  it('shows the teaser card (not a plain sentence) below the 640px breakpoint', () => {
    ro.setWidth(500)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.queryByTestId('collection-graph-canvas')).not.toBeInTheDocument()
    expect(
      screen.getByText(/interactive collection graph is best on a larger screen/i),
    ).toBeInTheDocument()
  })
})
