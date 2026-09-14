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

  it('keeps the per-type breakdown when nodes_truncated but the total is not larger (PSY-1476)', async () => {
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: {
        ...mockData,
        // Stale/skewed payload: flag set but node_total ≤ shown (3 nodes) — the
        // shared guard degrades, so the header keeps the breakdown + caption.
        collection: { ...mockData.collection, node_total: 3, nodes_truncated: true },
      },
      isLoading: false,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.getByText(/2 artists · 1 venue/)).toBeInTheDocument()
    expect(screen.queryByText(/Top 3 of/)).not.toBeInTheDocument()
    expect(screen.getByText(/Showing every item in this collection/)).toBeInTheDocument()
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

  // The teaser replaces the canvas, not the announced failure: a settled error
  // keeps its heading and its alert at every width, which is what the sibling
  // graph sections do and what an error a visitor is waiting on needs.
  it('keeps the error card and its heading below the 640px breakpoint', async () => {
    ro.setWidth(500)
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
    expect(screen.getByRole('alert')).toHaveTextContent(/couldn't load/i)
    expect(screen.queryByRole('link', { name: /music map/i })).not.toBeInTheDocument()
  })

  // An empty collection has no graph to link to, so the teaser stays away and
  // the section keeps the sentence that says what to add.
  it('keeps the empty-collection message below the 640px breakpoint', async () => {
    ro.setWidth(500)
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: { collection: { slug: 'desert-doom', artist_count: 0, edge_count: 0 }, nodes: [], links: [] },
      isLoading: false,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.getByText(/No items yet/)).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /music map/i })).not.toBeInTheDocument()
  })

  it('collapses to a one-line map teaser below the 640px breakpoint', () => {
    ro.setWidth(500)
    const { container } = renderWithProviders(
      <CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />,
    )
    expect(screen.queryByTestId('collection-graph-canvas')).not.toBeInTheDocument()

    // No section chrome: no heading, no breakdown line, no "needs a larger
    // screen" card, no in-page link-out.
    expect(screen.queryByText('Collection graph')).not.toBeInTheDocument()
    expect(screen.queryByText(/needs a larger screen/i)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /Browse the collection/i }),
    ).not.toBeInTheDocument()

    const links = screen.getAllByRole('link')
    expect(links).toHaveLength(1)
    expect(links[0]).toHaveAccessibleName(
      'See how this collection\u2019s artists connect on the music map',
    )
    // Gatecreeper and Spirit Adrift share the fixture's one edge, so the name
    // decides.
    expect(links[0]).toHaveAttribute('href', '/graph?artist=gatecreeper')

    // The `#graph` deep-link target survives the collapse (Cmd+K, PSY-366).
    expect(container.querySelector('#graph')).not.toBeNull()
  })

  // A sentence naming the collection has to land on a map that knows it.
  it('roots the teaser on the most connected artist in the collection', async () => {
    ro.setWidth(500)
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: {
        ...mockData,
        links: [
          ...mockData.links,
          { source_id: 2, target_id: 3, type: 'played_at', score: 0.4 },
        ],
      },
      isLoading: false,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    // Spirit Adrift now carries two edges against Gatecreeper's one.
    expect(screen.getByRole('link')).toHaveAttribute('href', '/graph?artist=spirit-adrift')
  })

  // The payload is mixed-type and the Observatory roots on artists only.
  it('falls back to the unrooted map for an artist-free collection', async () => {
    ro.setWidth(500)
    const hooks = await import('../hooks')
    vi.mocked(hooks.useCollectionGraph).mockReturnValue({
      data: {
        ...mockData,
        nodes: [
          { ...mockData.nodes[2], id: 1, entity_type: 'venue', slug: 'valley-bar-phoenix-az' },
          { ...mockData.nodes[2], id: 2, entity_type: 'release', slug: 'sonoran-surf' },
        ],
      },
      isLoading: false,
      isError: false,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<CollectionGraph slug="desert-doom" collectionTitle="Desert Doom" />)
    expect(screen.getByRole('link')).toHaveAttribute('href', '/graph')
  })
})
