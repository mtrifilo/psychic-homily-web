import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// ── Fixture ─────────────────────────────────────────────────────────────────
// Center 100 with 16 connected neighbors (ids 1..16, center-edge score
// descending with id) so the 14-cap drops ids 15 and 16, plus one
// cross-connection between kept neighbors and one to a dropped neighbor.

function neighbor(id: number): ArtistGraphNode {
  return {
    id,
    name: `Artist ${id}`,
    slug: `artist-${id}`,
    upcoming_show_count: 0,
    has_playable_audio: false,
  }
}

function centerLink(targetId: number, score: number): ArtistGraphLink {
  return {
    source_id: 100,
    target_id: targetId,
    type: 'shared_bills',
    score,
    votes_up: 0,
    votes_down: 0,
  }
}

const NEIGHBOR_COUNT = 16

const mockGraph: ArtistGraph = {
  center: {
    id: 100,
    name: 'Center Artist',
    slug: 'center-artist',
    upcoming_show_count: 0,
    has_playable_audio: false,
  },
  nodes: Array.from({ length: NEIGHBOR_COUNT }, (_, i) => neighbor(i + 1)),
  links: [
    ...Array.from({ length: NEIGHBOR_COUNT }, (_, i) =>
      centerLink(i + 1, 1 - i * 0.05)
    ),
    // Cross-connection between two kept neighbors — must survive the cap.
    { source_id: 1, target_id: 2, type: 'shared_label', score: 0.4, votes_up: 0, votes_down: 0 },
    // Cross-connection touching a dropped neighbor (16) — must be pruned.
    { source_id: 1, target_id: 16, type: 'shared_label', score: 0.4, votes_up: 0, votes_down: 0 },
  ],
}

vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: vi.fn(() => ({ data: mockGraph, isLoading: false })),
}))

vi.mock('../hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: vi.fn(() => ({ data: undefined, isError: false })),
}))

// Canvas can't render in jsdom. Stub the visualization, forwarding the props
// this section is responsible for wiring (height, tiers, focus pin, select)
// so the assertions exercise the section's contract, not the canvas.
vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: ({
    data,
    height,
    instantFit,
    labelTiers,
    focusNodeId,
    onSelect,
  }: {
    data: ArtistGraph
    height?: number
    instantFit?: boolean
    labelTiers?: readonly { fontSize: number }[]
    focusNodeId?: number | null
    onSelect?: (node: ArtistGraphNode) => void
  }) => (
    <div
      data-testid="connections-canvas"
      data-node-count={data.nodes.length}
      data-link-count={data.links.length}
      data-height={height ?? ''}
      data-instant-fit={String(instantFit ?? false)}
      data-tier-sizes={(labelTiers ?? []).map(t => t.fontSize).join(',')}
      data-focus-node={focusNodeId ?? ''}
    >
      {data.nodes.map(node => (
        <button key={node.id} onClick={() => onSelect?.(node)}>
          select-{node.id}
        </button>
      ))}
    </div>
  ),
}))

import {
  ArtistConnectionsSection,
  capEgoNeighbors,
  CONNECTIONS_NEIGHBOR_CAP,
  SIMILAR_ARTISTS_ANCHOR,
} from './ArtistConnectionsSection'
import { useArtistGraph } from '../hooks/useArtistGraph'

describe('capEgoNeighbors', () => {
  it('caps to the top N neighbors by max center-edge score', () => {
    const { graph, shown, total } = capEgoNeighbors(mockGraph, CONNECTIONS_NEIGHBOR_CAP)
    expect(shown).toBe(14)
    expect(total).toBe(16)
    expect(graph.nodes.map(n => n.id)).toEqual(
      Array.from({ length: 14 }, (_, i) => i + 1)
    )
  })

  it('keeps cross-links between kept neighbors and prunes links to dropped ones', () => {
    const { graph } = capEgoNeighbors(mockGraph, CONNECTIONS_NEIGHBOR_CAP)
    const cross = graph.links.filter(l => l.type === 'shared_label')
    expect(cross).toHaveLength(1)
    expect(cross[0]).toMatchObject({ source_id: 1, target_id: 2 })
    // 14 center links + 1 kept cross link.
    expect(graph.links).toHaveLength(15)
  })

  it('breaks score ties on ascending id (the sidebar order)', () => {
    const tied: ArtistGraph = {
      center: mockGraph.center,
      nodes: [neighbor(3), neighbor(1), neighbor(2)],
      links: [centerLink(3, 0.5), centerLink(1, 0.5), centerLink(2, 0.5)],
    }
    const { graph } = capEgoNeighbors(tied, 2)
    expect(graph.nodes.map(n => n.id)).toEqual([1, 2])
  })

  it('excludes neighbors with no center edge from both the cap and the total', () => {
    const withOrphan: ArtistGraph = {
      center: mockGraph.center,
      nodes: [neighbor(1), neighbor(2), neighbor(3)],
      links: [
        centerLink(1, 0.9),
        centerLink(2, 0.8),
        // 3 only cross-connects to 1 — not center-connected.
        { source_id: 1, target_id: 3, type: 'shared_label', score: 0.4, votes_up: 0, votes_down: 0 },
      ],
    }
    const { graph, shown, total } = capEgoNeighbors(withOrphan, 14)
    expect(shown).toBe(2)
    expect(total).toBe(2)
    expect(graph.nodes.map(n => n.id)).toEqual([1, 2])
  })
})

describe('ArtistConnectionsSection', () => {
  let resizeObserver: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    resizeObserver = installImmediateResizeObserver()
    // Re-seed per test so an override can't leak (no test-order coupling).
    vi.mocked(useArtistGraph).mockImplementation(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      () => ({ data: mockGraph, isLoading: false }) as any
    )
  })

  afterEach(() => {
    resizeObserver.restore()
  })

  function renderSection(onExpand = vi.fn()) {
    renderWithProviders(
      <ArtistConnectionsSection
        artistId={100}
        artistName="Center Artist"
        onExpand={onExpand}
      />
    )
    return onExpand
  }

  it('renders the header, truncated count line, and the capped 360px canvas', () => {
    renderSection()
    expect(screen.getByRole('heading', { name: 'Connections' })).toBeInTheDocument()
    expect(
      screen.getByText(/Top 14 of 16 connected artists · click a name to see how it connects/)
    ).toBeInTheDocument()
    const canvas = screen.getByTestId('connections-canvas')
    expect(canvas).toHaveAttribute('data-node-count', '14')
    expect(canvas).toHaveAttribute('data-height', '360')
    // Section-class ladder 14/11/9 (SECTION_LABEL_TIERS).
    expect(canvas).toHaveAttribute('data-tier-sizes', '14,11,9')
  })

  it('frames the camera instantly — an inline section must not tween on first paint', () => {
    renderSection()
    expect(screen.getByTestId('connections-canvas')).toHaveAttribute(
      'data-instant-fit',
      'true'
    )
  })

  it('renders a plain count line when the payload fits under the cap', () => {
    const small: ArtistGraph = {
      center: mockGraph.center,
      nodes: [neighbor(1), neighbor(2)],
      links: [centerLink(1, 0.9), centerLink(2, 0.8)],
    }
    vi.mocked(useArtistGraph).mockImplementation(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      () => ({ data: small, isLoading: false }) as any
    )
    renderSection()
    expect(
      screen.getByText(/2 connected artists · click a name to see how it connects/)
    ).toBeInTheDocument()
    expect(screen.queryByText(/top \d+ of \d+/i)).not.toBeInTheDocument()
  })

  it('hides entirely when the artist has no relationships', () => {
    vi.mocked(useArtistGraph).mockImplementation(
      () =>
        ({
          data: { center: mockGraph.center, nodes: [], links: [] },
          isLoading: false,
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
        }) as any
    )
    renderSection()
    expect(screen.queryByRole('heading', { name: 'Connections' })).not.toBeInTheDocument()
    expect(screen.queryByTestId('connections-canvas')).not.toBeInTheDocument()
  })

  it('fires onExpand from the [Expand] header action', () => {
    const onExpand = renderSection()
    fireEvent.click(screen.getByRole('button', { name: 'Expand' }))
    expect(onExpand).toHaveBeenCalledTimes(1)
  })

  it('renders the teaser card (no canvas) below the 640px gate, linking to the sidebar anchor', () => {
    resizeObserver.setWidth(390)
    renderSection()
    expect(screen.queryByTestId('connections-canvas')).not.toBeInTheDocument()
    const link = screen.getByRole('link', { name: /see similar artists/i })
    expect(link).toHaveAttribute('href', `#${SIMILAR_ARTISTS_ANCHOR}`)
    // The copy points at [Expand], so pin the two together: unlike the sibling
    // graph surfaces, this section keeps [Expand] visible below the gate, and
    // ArtistGraphDialog does draw a canvas there. Gating it would strand the copy.
    expect(
      screen.getByText(/Best on a larger screen, or open the full map\./)
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand' })).toBeInTheDocument()
    // The count line still discloses scale on mobile.
    expect(screen.getByText(/Top 14 of 16 connected artists/)).toBeInTheDocument()
  })

  it('selects a node into the context panel, pins focus, and deselects on second click', () => {
    renderSection()
    fireEvent.click(screen.getByRole('button', { name: 'select-1' }))
    // Panel names the selected artist; focus pin threads to the canvas.
    expect(screen.getByText('Artist 1')).toBeInTheDocument()
    expect(screen.getByTestId('connections-canvas')).toHaveAttribute('data-focus-node', '1')
    // Second click on the same node deselects (shared toggle grammar).
    fireEvent.click(screen.getByRole('button', { name: 'select-1' }))
    expect(screen.queryByText('Artist 1')).not.toBeInTheDocument()
    expect(screen.getByTestId('connections-canvas')).toHaveAttribute('data-focus-node', '')
  })

  it('ignores clicks on the center node (no self-panel)', () => {
    renderSection()
    // The stub renders buttons only for the capped neighbor nodes, so drive
    // the resolver directly through a neighbor select then a center select.
    fireEvent.click(screen.getByRole('button', { name: 'select-1' }))
    expect(screen.getByText('Artist 1')).toBeInTheDocument()
    // No center button exists in the stub (center isn't in data.nodes);
    // resolveNode excludes it by construction — assert the capped payload
    // never contains the center as a selectable node.
    expect(screen.queryByRole('button', { name: 'select-100' })).not.toBeInTheDocument()
  })
})
