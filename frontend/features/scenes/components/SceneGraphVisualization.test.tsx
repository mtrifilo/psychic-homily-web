import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { SceneGraphResponse } from '../types'
import type { GraphNode } from '@/components/graph/ForceGraphView'

// PSY-690: SceneGraphVisualization is a thin shape adapter over the shared
// ForceGraphView. Its own responsibilities are: (1) compose the scene-specific
// aria-label, (2) own the node-select → ArtistContextPanel wiring (PSY-1451 —
// click selects into the panel; navigation only via the panel's "Open page →"),
// and (3) forward props (data, width, height, hiddenClusterIDs) through
// unchanged.
//
// jsdom can't render the d3/canvas pipeline (ForceGraphView dynamic-imports
// react-force-graph-2d with ssr:false), so we mock ForceGraphView and assert
// on the props the wrapper hands it — this both smoke-mounts the wrapper
// without throwing and verifies its adapter logic. The ArtistContextPanel
// renders REAL (it's plain DOM), so the Esc path exercises the actual Radix
// DismissableLayer contract (PSY-1355). The reduced-motion simulation pause
// lives inside ForceGraphView, not this wrapper, so it's out of scope here.

// Capture the props ForceGraphView receives and expose node/background
// clicks as buttons so tests can drive the selection path (HomeSceneGraph
// test pattern).
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

// Card fetch is the panel caller's concern — stub the hook (HomeSceneGraph
// test precedent) so no QueryClient/network is needed and the artistId /
// enabled contract is assertable.
const useArtistGraphCard = vi.fn()
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (opts: { artistId: number | null; enabled?: boolean }) =>
    useArtistGraphCard(opts),
}))

import { SceneGraphVisualization } from './SceneGraphVisualization'
import { graphSelectGestureHint } from '@/components/graph/ArtistContextPanel'

const data: SceneGraphResponse = {
  scene: {
    slug: 'phoenix-az',
    city: 'Phoenix',
    state: 'AZ',
    // artist_count mirrors nodes.length below — same backend contract the
    // SceneGraph.test fixture honors (ArtistCount: len(rows)); edge_count
    // mirrors the empty links list.
    artist_count: 1,
    edge_count: 0,
    metro_roster_total: 1,
    roster_truncated: false,
  },
  clusters: [{ id: 'v_1', label: 'Valley Bar', size: 6, color_index: 0 }],
  nodes: [
    {
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'v_1',
      is_isolate: false,
      has_playable_audio: false,
    },
  ],
  links: [],
}

// The shared select-gesture sentence appended to every scene aria-label.
const SELECT_HINT = ` ${graphSelectGestureHint}`

describe('SceneGraphVisualization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    lastProps = null
    useArtistGraphCard.mockReturnValue({ data: undefined, isError: false })
  })

  it('smoke-mounts without throwing and renders the graph view element', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    expect(screen.getByTestId('force-graph-view')).toBeInTheDocument()
  })

  it('composes the scene-specific aria-label from the scene metadata', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    const view = screen.getByTestId('force-graph-view')
    expect(view).toHaveAttribute(
      'aria-label',
      `Scene relationship graph for Phoenix, AZ: 1 artist, 0 connections.${SELECT_HINT}`
    )
  })

  it('singularizes a one-edge graph in the aria-label ("1 connection")', () => {
    render(
      <SceneGraphVisualization
        data={{ ...data, scene: { ...data.scene, edge_count: 1 } }}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    expect(screen.getByTestId('force-graph-view')).toHaveAttribute(
      'aria-label',
      `Scene relationship graph for Phoenix, AZ: 1 artist, 1 connection.${SELECT_HINT}`
    )
  })

  // PSY-1296: assistive tech must hear the same "top N of M" framing the
  // visual header shows when the PSY-1277 roster cap truncated the node set.
  it('describes a truncated graph honestly in the aria-label', () => {
    render(
      <SceneGraphVisualization
        data={{
          ...data,
          scene: {
            ...data.scene,
            metro_roster_total: 90,
            roster_truncated: true,
          },
        }}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    expect(screen.getByTestId('force-graph-view')).toHaveAttribute(
      'aria-label',
      `Scene relationship graph for Phoenix, AZ: top 1 of 90 artists, 0 connections.${SELECT_HINT}`
    )
  })

  it('forwards graph payload, width, and hidden clusters to ForceGraphView', () => {
    const hidden = new Set(['v_1'])
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={777}
        hiddenClusterIDs={hidden}
        height={500}
      />
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
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    expect(lastProps!.height).toBeUndefined()
  })

  it('opts into the accessible node controls so the advertised select gesture has a keyboard path', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    expect(lastProps!.showAccessibleNodeControls).toBe(true)
  })

  // ── PSY-1451: node click selects into the context panel ──

  it('node click opens the context panel and fetches that artist’s card; second click deselects', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' })
    ).toBeInTheDocument()
    expect(useArtistGraphCard).toHaveBeenLastCalledWith(
      expect.objectContaining({ artistId: 1, enabled: true })
    )
    // Navigation lives ONLY on the panel's "Open page →" link.
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/gatecreeper'
    )
    // Second click on the same node puts the panel away.
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
  })

  it('background click dismisses the panel and focus returns to the canvas wrap', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).toBe(canvasWrap)
  })

  it('background click with no panel open is a no-op (no focus steal)', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).not.toBe(canvasWrap)
  })

  it('Escape closes the panel (real DismissableLayer) and claims the keypress from outer layers', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' })
    ).toBeInTheDocument()
    // fireEvent returns false when the event was defaultPrevented — Radix
    // preventDefaults in the capture phase, which is exactly what makes the
    // fullscreen overlay's own Esc listener skip this press (PSY-1355).
    const notPrevented = fireEvent.keyDown(document, { key: 'Escape' })
    expect(notPrevented).toBe(false)
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).toBe(canvasWrap)
    // With the panel closed, the NEXT Esc goes through un-prevented — this
    // is the press the fullscreen overlay's listener acts on (panel closes
    // first, overlay second).
    expect(fireEvent.keyDown(document, { key: 'Escape' })).toBe(true)
  })

  it('deselects when an edge click opens the connection inspector (panels never stack)', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' })
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'edge-inspect-open' }))
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
  })

  it('closes the panel when the selected node’s empty cluster_id falls into a hidden "other"', () => {
    const otherData = {
      ...data,
      nodes: [{ ...data.nodes[0], cluster_id: '' }],
    }
    const { rerender } = render(
      <SceneGraphVisualization
        data={otherData}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' })
    ).toBeInTheDocument()

    rerender(
      <SceneGraphVisualization
        data={otherData}
        containerWidth={1024}
        hiddenClusterIDs={new Set(['other'])}
      />
    )
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
  })

  it('closes the panel when the selected node’s cluster is hidden via the legend', () => {
    const { rerender } = render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' })
    ).toBeInTheDocument()

    rerender(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set(['v_1'])}
      />
    )
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
  })

  it('closes the panel when a data refresh drops the selected node', () => {
    const { rerender } = render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(
      screen.getByRole('region', { name: 'About Gatecreeper' })
    ).toBeInTheDocument()

    rerender(
      <SceneGraphVisualization
        data={{ ...data, nodes: [] }}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    expect(
      screen.queryByRole('region', { name: 'About Gatecreeper' })
    ).toBeNull()
  })
})
