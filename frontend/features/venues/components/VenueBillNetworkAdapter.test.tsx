import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1455: the venue bill network is a Section-class graph surface, so it
// ships the click-to-inspect ConnectionPanel (locked grammar, 2026-07-11).
// These tests mount the REAL ForceGraphView through the venue adapter (only
// the canvas lib is stubbed — jsdom can't render it) so they exercise the
// venue edge-click path end-to-end: typed shared_bills edge click → panel
// opens for the pair → provenance fetch fires for the artist pair.
// Mirrors the stub pattern in ForceGraphView.connectionPanel.test.tsx.

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
    zoomToFit: vi.fn(),
    zoom: vi.fn(() => 1),
    centerAt: vi.fn(() => ({ x: 0, y: 0 })),
    getGraphBbox: vi.fn(() => ({ x: [-100, 100], y: [-100, 100] })),
  },
  lastProps: { value: null as Record<string, unknown> | null },
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(
        props: Record<string, unknown>,
        ref: React.Ref<unknown>,
      ) {
        React.useImperativeHandle(ref, () => h.graph)
        React.useEffect(() => {
          h.lastProps.value = props
        })
        return React.createElement('canvas', { 'data-testid': 'stub-canvas' })
      }),
  }
})

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => false,
}))

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Opening the panel fetches provenance (PSY-1335). Reject by default (404 =
// no stored connection → rows stay text-only); the resolve test asserts the
// entity upgrade for a venue pair.
const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))
vi.mock('@/lib/api-base', () => ({
  API_BASE_URL: 'http://localhost:8080',
}))

import { SceneGraphVisualizationStyleAdapter } from './VenueBillNetworkAdapter'
import type { VenueBillNetworkResponse } from '../types'

// Shape mirrors the backend venue_bill_network.go contract: typed
// shared_bills links with the DeriveSharedBills detail blob.
const data: VenueBillNetworkResponse = {
  venue: {
    id: 1,
    slug: 'valley-bar-phoenix-az',
    name: 'Valley Bar',
    city: 'Phoenix',
    state: 'AZ',
    artist_count: 3,
    artist_total: 3,
    roster_truncated: false,
    edge_count: 2,
    show_count: 25,
    window: 'all_time',
  },
  clusters: [],
  nodes: [
    {
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: false,
      at_venue_show_count: 5,
    },
    {
      id: 2,
      name: 'Sundressed',
      slug: 'sundressed',
      upcoming_show_count: 1,
      cluster_id: 'other',
      is_isolate: false,
      at_venue_show_count: 4,
    },
    {
      id: 3,
      name: 'Numb Bats',
      slug: 'numb-bats',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: false,
      at_venue_show_count: 3,
    },
  ],
  links: [
    {
      source_id: 1,
      target_id: 2,
      type: 'shared_bills',
      score: 0.5,
      detail: { shared_count: 5, last_shared: '2026-05-14' },
      is_cross_cluster: false,
    },
    {
      source_id: 2,
      target_id: 3,
      type: 'shared_bills',
      score: 0.3,
      detail: { shared_count: 3 },
      is_cross_cluster: false,
    },
  ],
}

const renderAdapter = () =>
  renderWithProviders(
    <SceneGraphVisualizationStyleAdapter
      data={data}
      venueName="Valley Bar"
      containerWidth={800}
    />,
  )

const clickLink = (source: number, target: number, type = 'shared_bills') => {
  act(() => {
    ;(h.lastProps.value!.onLinkClick as (l: unknown) => void)({ source, target, type })
  })
}

beforeEach(() => {
  h.lastProps.value = null
  vi.clearAllMocks()
  mockApiRequest.mockRejectedValue(Object.assign(new Error('Not Found'), { status: 404 }))
})

describe('VenueBillNetworkAdapter connection panel (PSY-1455)', () => {
  it('opens the ConnectionPanel on a shared-bills edge click, without navigating', () => {
    renderAdapter()
    clickLink(1, 2)
    const panel = screen.getByRole('region', {
      name: 'Why Gatecreeper and Sundressed are connected',
    })
    // The venue payload's typed row renders from graph data (phase-1 text
    // row). Scoped to the panel — the adapter's EdgeLegend also says
    // "Shared Bills".
    expect(within(panel).getByText('Shared Bills')).toBeInTheDocument()
    // Edge clicks inspect; nothing on this surface navigates directly —
    // node clicks select into the ArtistContextPanel (PSY-1451 grammar).
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('fetches pair provenance for the venue pair and upgrades rows to entity links', async () => {
    mockApiRequest.mockResolvedValue({
      connections: [
        {
          type: 'shared_bills',
          score: 0.5,
          entities: [
            {
              kind: 'show',
              id: 9,
              slug: 'gatecreeper-valley-bar',
              name: 'Valley Bar',
              date: '2026-05-14',
            },
          ],
          entity_total: 5,
        },
      ],
    })
    renderAdapter()
    clickLink(1, 2)

    // Venue nodes are artist IDs, so the shared artist-pair provenance
    // endpoint resolves for venue-scope edges — canonical (sorted) orientation.
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/artists/1/relationships/2/provenance',
      { method: 'GET' },
    )
    expect(
      await screen.findByRole('link', { name: '2026-05-14 · Valley Bar' }),
    ).toHaveAttribute('href', '/shows/gatecreeper-valley-bar')
  })

  it('claims Escape from the fullscreen overlay layer (panel closes first)', () => {
    renderAdapter()
    clickLink(2, 3)
    expect(screen.getByRole('region', { name: /connected/ })).toBeInTheDocument()
    // The panel preventDefaults Escape in the capture phase; the fullscreen
    // overlay's bubble listener skips defaultPrevented — so with the panel
    // open, Esc closes the panel and leaves the overlay intact (PSY-1355).
    const outerEscapes: boolean[] = []
    const outerListener = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented) outerEscapes.push(true)
    }
    document.addEventListener('keydown', outerListener)
    fireEvent.keyDown(document.body, { key: 'Escape' })
    document.removeEventListener('keydown', outerListener)
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
    expect(outerEscapes).toHaveLength(0)
  })
})
