import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { VenueBillNetworkResponse } from '../types'
import type { GraphNode } from '@/components/graph/ForceGraphView'

// PSY-1451: the venue bill network follows the locked Section-class grammar —
// a node click SELECTS into the shared ArtistContextPanel; navigation happens
// only via the panel's "Open page →". jsdom can't render the d3/canvas
// pipeline, so mock ForceGraphView and assert on the props the adapter hands
// it (StationGraphVisualization.test.tsx pattern). The ArtistContextPanel
// renders REAL (plain DOM), so the Esc path exercises the actual Radix
// DismissableLayer contract (PSY-1355). The edge-click ConnectionPanel path
// against the REAL ForceGraphView lives in VenueBillNetworkAdapter.test.tsx.

// Capture the props ForceGraphView receives and expose node/background/edge
// clicks as buttons so tests can drive the selection path.
interface CapturedProps {
  nodes: Array<{ id: number; name: string; slug: string }>
  links: unknown
  clusters: unknown
  containerWidth: number
  height?: number
  ariaLabel: string
  onNodeClick: (node: GraphNode) => void
  onBackgroundClick?: () => void
  onConnectionInspectOpen?: () => void
  showAccessibleNodeControls?: boolean
  showEdgeLegend?: boolean
  showConnectionPanel?: boolean
}
let lastProps: CapturedProps | null = null

vi.mock('@/components/graph/ForceGraphView', () => ({
  OTHER_CLUSTER_ID: 'other',
  ForceGraphView: (props: CapturedProps) => {
    lastProps = props
    return (
      <div data-testid="force-graph-view" aria-label={props.ariaLabel} role="img">
        {props.nodes.map(n => (
          <button
            key={n.id}
            type="button"
            onClick={() => props.onNodeClick(n as GraphNode)}
          >
            {`node-${n.slug}`}
          </button>
        ))}
        <button type="button" onClick={() => props.onBackgroundClick?.()}>
          canvas-background
        </button>
        <button type="button" onClick={() => props.onConnectionInspectOpen?.()}>
          edge-inspect-open
        </button>
      </div>
    )
  },
}))

// Card fetch is the panel caller's concern — stub the hook (scene/station
// test precedent) so no QueryClient/network is needed and the artistId /
// enabled contract is assertable.
const useArtistGraphCard = vi.fn()
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (opts: { artistId: number | null; enabled?: boolean }) =>
    useArtistGraphCard(opts),
}))

import { SceneGraphVisualizationStyleAdapter } from './VenueBillNetworkAdapter'
import { graphSelectGestureHint } from '@/components/graph/ArtistContextPanel'

const data: VenueBillNetworkResponse = {
  venue: {
    id: 1,
    slug: 'valley-bar-phoenix-az',
    name: 'Valley Bar',
    city: 'Phoenix',
    state: 'AZ',
    artist_count: 8,
    artist_total: 8,
    roster_truncated: false,
    edge_count: 5,
    show_count: 25,
    window: 'last_12m',
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
  ],
  links: [],
}

const renderAdapter = (override?: Partial<VenueBillNetworkResponse>) =>
  render(
    <SceneGraphVisualizationStyleAdapter
      data={{ ...data, ...override }}
      venueName="Valley Bar"
      containerWidth={1024}
    />,
  )

describe('VenueBillNetworkAdapter selection (PSY-1451)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    lastProps = null
    useArtistGraphCard.mockReturnValue({ data: undefined, isError: false })
  })

  it('composes the venue aria-label, ending with the shared select-gesture hint', () => {
    renderAdapter()
    expect(screen.getByTestId('force-graph-view')).toHaveAttribute(
      'aria-label',
      `Co-bill network for Valley Bar (last 12 months): 8 artists, 5 co-bills. ${graphSelectGestureHint}`,
    )
  })

  it('forwards graph payload, width, height, and the shared-grammar opt-ins', () => {
    render(
      <SceneGraphVisualizationStyleAdapter
        data={data}
        venueName="Valley Bar"
        containerWidth={777}
        height={500}
      />,
    )
    expect(lastProps).not.toBeNull()
    expect(lastProps!.nodes).toBe(data.nodes)
    expect(lastProps!.links).toBe(data.links)
    expect(lastProps!.clusters).toBe(data.clusters)
    expect(lastProps!.containerWidth).toBe(777)
    expect(lastProps!.height).toBe(500)
    expect(lastProps!.showEdgeLegend).toBe(true)
    expect(lastProps!.showConnectionPanel).toBe(true)
    expect(lastProps!.showAccessibleNodeControls).toBe(true)
  })

  it('node click opens the context panel and fetches that artist’s card; second click deselects', () => {
    renderAdapter()
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()
    expect(useArtistGraphCard).toHaveBeenLastCalledWith(
      expect.objectContaining({ artistId: 1, enabled: true }),
    )
    // Navigation lives ONLY on the panel's "Open page →" link.
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/gatecreeper',
    )
    // Second click on the same node puts the panel away.
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
  })

  it('background click dismisses the panel and focus returns to the canvas wrap', () => {
    renderAdapter()
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).toBe(canvasWrap)
  })

  it('background click with no panel open is a no-op (no focus steal)', () => {
    renderAdapter()
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).not.toBe(canvasWrap)
  })

  it('Escape closes the panel (real DismissableLayer) and claims the keypress from outer layers', () => {
    renderAdapter()
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()
    // fireEvent returns false when the event was defaultPrevented — Radix
    // preventDefaults in the capture phase, which is exactly what makes the
    // fullscreen overlay's own Esc listener skip this press (PSY-1355):
    // panel closes first, overlay second.
    const notPrevented = fireEvent.keyDown(document, { key: 'Escape' })
    expect(notPrevented).toBe(false)
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).toBe(canvasWrap)
    // With the panel closed, the NEXT Esc goes through un-prevented — this
    // is the press the fullscreen overlay's listener acts on.
    expect(fireEvent.keyDown(document, { key: 'Escape' })).toBe(true)
  })

  it('deselects when an edge click opens the connection inspector (panels never stack)', () => {
    renderAdapter()
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'edge-inspect-open' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
  })

  it('closes the panel when a window-filter refetch drops the selected node', () => {
    const { rerender } = renderAdapter()
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()

    rerender(
      <SceneGraphVisualizationStyleAdapter
        data={{ ...data, nodes: [] }}
        venueName="Valley Bar"
        containerWidth={1024}
      />,
    )
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
  })
})
