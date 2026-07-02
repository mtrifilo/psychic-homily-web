import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { RadioStationGraphResponse } from '../types'
import type { GraphNode } from '@/components/graph/ForceGraphView'

// StationGraphVisualization is a thin shape adapter over the shared
// ForceGraphView (same contract as SceneGraphVisualization): compose the
// station aria-label, translate node click → artist navigation, forward
// props unchanged. jsdom can't render the d3/canvas pipeline, so mock
// ForceGraphView and assert on the props the wrapper hands it.

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

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

import { StationGraphVisualization } from './StationGraphVisualization'

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

  it('composes the station aria-label from the station metadata', () => {
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
      'Airplay graph for KEXP: 12 artists, 4 connections. Use the shows and playlists lists to browse without the canvas.',
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

  it('navigates to the artist page when a node is clicked', () => {
    render(
      <StationGraphVisualization
        data={data}
        containerWidth={1024}
        hiddenClusterIDs={new Set()}
      />,
    )
    lastProps!.onNodeClick({
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
    })
    expect(mockPush).toHaveBeenCalledWith('/artists/gatecreeper')
  })
})
