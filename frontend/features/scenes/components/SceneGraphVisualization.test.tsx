import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { SceneGraphResponse } from '../types'
import type { GraphNode } from '@/components/graph/ForceGraphView'

// PSY-690: SceneGraphVisualization is a thin shape adapter over the shared
// ForceGraphView. Its own responsibilities are: (1) compose the scene-specific
// aria-label, (2) hold the next-router dependency and translate a node click
// into navigation to that artist, and (3) forward props (data, width, height,
// hiddenClusterIDs) through unchanged.
//
// jsdom can't render the d3/canvas pipeline (ForceGraphView dynamic-imports
// react-force-graph-2d with ssr:false), so we mock ForceGraphView and assert
// on the props the wrapper hands it — this both smoke-mounts the wrapper
// without throwing and verifies its adapter logic. The reduced-motion
// simulation pause lives inside ForceGraphView, not this wrapper, so it's out
// of scope here.

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Capture the props ForceGraphView receives and expose its onNodeClick so we
// can drive the navigation path.
interface CapturedProps {
  nodes: unknown
  links: unknown
  clusters: unknown
  containerWidth: number
  height?: number
  hiddenClusterIDs: Set<string>
  ariaLabel: string
  onNodeClick: (node: GraphNode) => void
}
let lastProps: CapturedProps | null = null

vi.mock('@/components/graph/ForceGraphView', () => ({
  ForceGraphView: (props: CapturedProps) => {
    lastProps = props
    return (
      <div data-testid="force-graph-view" aria-label={props.ariaLabel} role="img" />
    )
  },
}))

import { SceneGraphVisualization } from './SceneGraphVisualization'

const data: SceneGraphResponse = {
  scene: {
    slug: 'phoenix-az',
    city: 'Phoenix',
    state: 'AZ',
    artist_count: 12,
    edge_count: 4,
    metro_roster_total: 12,
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
    },
  ],
  links: [],
}

describe('SceneGraphVisualization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    lastProps = null
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
      'Scene relationship graph for Phoenix, AZ: 12 artists, 4 connections.'
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
      'Scene relationship graph for Phoenix, AZ: showing top 12 of 90 artists, 4 connections.'
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

  it('navigates to the artist page when a node is clicked', () => {
    render(
      <SceneGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />
    )
    // Drive the click handler the wrapper handed to ForceGraphView.
    lastProps!.onNodeClick({
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
    })
    expect(mockPush).toHaveBeenCalledWith('/artists/gatecreeper')
  })
})
