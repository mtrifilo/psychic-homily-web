import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { RadioStationGraphResponse } from '../types'
import type { GraphNode } from '@/components/graph/ForceGraphView'

// StationGraphVisualization is a thin shape adapter over the shared
// ForceGraphView (same contract as SceneGraphVisualization): compose the
// station aria-label, own the node-select → ArtistContextPanel wiring
// (PSY-1451 — click selects into the panel; navigation only via the panel's
// "Open page →"), and forward props unchanged. jsdom can't render the
// d3/canvas pipeline, so mock ForceGraphView and assert on the props the
// wrapper hands it. The ArtistContextPanel renders REAL (plain DOM), so the
// Esc path exercises the actual Radix DismissableLayer contract (PSY-1355).

// Capture the props ForceGraphView receives and expose node/background/edge
// clicks as buttons so tests can drive the selection path (scene test
// pattern).
interface CapturedProps {
  nodes: Array<{ id: number; name: string; slug: string }>
  links: unknown
  clusters: unknown
  containerWidth: number
  height?: number
  hiddenClusterIDs: Set<string>
  ariaLabel: string
  onNodeClick: (node: GraphNode) => void
  onBackgroundClick?: () => void
  onConnectionInspectOpen?: () => void
  showAccessibleNodeControls?: boolean
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

// Card fetch is the panel caller's concern — stub the hook (scene test
// precedent) so no QueryClient/network is needed and the artistId / enabled
// contract is assertable.
const useArtistGraphCard = vi.fn()
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (opts: { artistId: number | null; enabled?: boolean }) =>
    useArtistGraphCard(opts),
}))

import { StationGraphVisualization } from './StationGraphVisualization'
import { graphSelectGestureHint } from '@/components/graph/ArtistContextPanel'

const data: RadioStationGraphResponse = {
  station: {
    id: 1,
    slug: 'kexp',
    name: 'KEXP',
    artist_count: 12,
    edge_count: 4,
    window: 'last_12m',
  },
  clusters: [{ id: 'rs_1', label: 'The Morning Show', size: 6, color_index: 0 }],
  nodes: [
    {
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'rs_1',
      is_isolate: false,
      play_count: 9,
    },
  ],
  links: [],
}

describe('StationGraphVisualization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    lastProps = null
    useArtistGraphCard.mockReturnValue({ data: undefined, isError: false })
  })

  it('smoke-mounts without throwing and renders the graph view element', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    expect(screen.getByTestId('force-graph-view')).toBeInTheDocument()
  })

  it('composes the station aria-label, ending with the shared select-gesture hint', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    const view = screen.getByTestId('force-graph-view')
    expect(view).toHaveAttribute(
      'aria-label',
      `Airplay graph for KEXP: 12 artists, 4 connections. Use the shows and playlists lists to browse without the canvas. ${graphSelectGestureHint}`,
    )
  })

  it('forwards graph payload, width, and hidden clusters to ForceGraphView', () => {
    const hidden = new Set(['rs_1'])
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={777}
        hiddenClusterIDs={hidden}
        height={500}
      />,
    )
    expect(lastProps).not.toBeNull()
    expect(lastProps!.nodes).toBe(data.nodes)
    expect(lastProps!.links).toBe(data.links)
    expect(lastProps!.clusters).toBe(data.clusters)
    expect(lastProps!.containerWidth).toBe(777)
    expect(lastProps!.height).toBe(500)
    expect(lastProps!.hiddenClusterIDs).toBe(hidden)
  })

  it('omits height when not provided so ForceGraphView applies its default', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    expect(lastProps!.height).toBeUndefined()
  })

  it('opts into the accessible node controls so the advertised select gesture has a keyboard path', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    expect(lastProps!.showAccessibleNodeControls).toBe(true)
  })

  // ── PSY-1451: node click selects into the context panel ──

  it('node click opens the context panel and fetches that artist’s card; second click deselects', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
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
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).toBe(canvasWrap)
  })

  it('background click with no panel open is a no-op (no focus steal)', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).not.toBe(canvasWrap)
  })

  it('Escape closes the panel (real DismissableLayer) and claims the keypress from outer layers', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
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
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'edge-inspect-open' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
  })

  it('closes the panel when the selected node’s cluster is hidden via a legend pill', () => {
    const { rerender } = render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()

    rerender(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set(['rs_1'])}
      />,
    )
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
  })

  it('closes the panel when a data refresh drops the selected node', () => {
    const { rerender } = render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' }),
    ).toBeInTheDocument()

    rerender(
      <StationGraphVisualization
        data={{ ...data, nodes: [] }}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' }),
    ).toBeNull()
  })
})
