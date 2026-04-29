import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { SceneGraphResponse } from '../types'

// Mock the useSceneGraph hook before the SceneGraph component imports it.
const mockData: SceneGraphResponse = {
  scene: {
    slug: 'phoenix-az',
    city: 'Phoenix',
    state: 'AZ',
    artist_count: 12,
    edge_count: 4,
  },
  clusters: [
    { id: 'v_1', label: 'Valley Bar', size: 6, color_index: 0 },
    { id: 'v_2', label: 'Crescent Ballroom', size: 6, color_index: 1 },
  ],
  nodes: [
    {
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'v_1',
      is_isolate: false,
    },
    {
      id: 2,
      name: 'Sundressed',
      slug: 'sundressed',
      upcoming_show_count: 1,
      cluster_id: 'v_1',
      is_isolate: false,
    },
    {
      id: 3,
      name: 'Numb Bats',
      slug: 'numb-bats',
      upcoming_show_count: 0,
      cluster_id: 'v_2',
      is_isolate: false,
    },
    {
      id: 4,
      name: 'Lonely Lounge',
      slug: 'lonely-lounge',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: true,
    },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'shared_bills', score: 0.5, is_cross_cluster: false },
    { source_id: 1, target_id: 3, type: 'shared_bills', score: 0.3, is_cross_cluster: true },
    { source_id: 2, target_id: 3, type: 'shared_bills', score: 0.2, is_cross_cluster: true },
    { source_id: 1, target_id: 3, type: 'shared_label', score: 0.4, is_cross_cluster: true },
  ],
}

vi.mock('../hooks/useScenes', () => ({
  useSceneGraph: vi.fn(() => ({
    data: mockData,
    isLoading: false,
    error: null,
  })),
}))

// Canvas can't render in jsdom. Stub the visualization so we can assert toggling.
vi.mock('./SceneGraphVisualization', () => ({
  SceneGraphVisualization: ({
    hiddenClusterIDs,
  }: {
    hiddenClusterIDs: Set<string>
  }) => (
    <div
      data-testid="scene-graph-canvas"
      data-hidden-clusters={Array.from(hiddenClusterIDs).sort().join(',')}
    >
      Scene Graph Canvas
    </div>
  ),
}))

import { SceneGraph } from './SceneGraph'

// Same ResizeObserver shim pattern as BillComposition.test.tsx so we can
// drive the >= 640px graph gate.
let mockContainerWidth = 1024

function setMockContainerWidth(width: number) {
  mockContainerWidth = width
}

class ImmediateResizeObserver {
  private callback: ResizeObserverCallback
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }
  observe(target: Element): void {
    this.callback(
      [
        {
          target,
          contentRect: { width: mockContainerWidth } as DOMRectReadOnly,
        } as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    )
  }
  unobserve(): void {}
  disconnect(): void {}
}

describe('SceneGraph', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const originalResizeObserver = (window as any).ResizeObserver

  beforeEach(() => {
    setMockContainerWidth(1024)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = ImmediateResizeObserver
  })

  afterEach(() => {
    setMockContainerWidth(1024)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = originalResizeObserver
  })

  it('renders the section header and counts', () => {
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
    expect(screen.getByText('Scene graph')).toBeInTheDocument()
    expect(screen.getByText(/4 artists/)).toBeInTheDocument()
    expect(screen.getByText(/4 connections/)).toBeInTheDocument()
    expect(screen.getByText(/1 unconnected/)).toBeInTheDocument()
  })

  it('hides the canvas below the 640px breakpoint', () => {
    setMockContainerWidth(500)
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
    // PSY-516: header copy is gated by `nodeCount === 0`, not by mobile gating,
    // so it may still render. The canvas + cluster legend must be absent.
    expect(screen.queryByTestId('scene-graph-canvas')).not.toBeInTheDocument()
    expect(screen.queryByText(/Valley Bar \(6\)/)).not.toBeInTheDocument()
  })

  it('renders canvas + cluster legend at desktop width', () => {
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
    expect(screen.getByTestId('scene-graph-canvas')).toBeInTheDocument()
    expect(screen.getByText(/Valley Bar \(6\)/)).toBeInTheDocument()
    expect(screen.getByText(/Crescent Ballroom \(6\)/)).toBeInTheDocument()
  })

  it('toggles cluster visibility when a legend pill is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

    const canvasBefore = screen.getByTestId('scene-graph-canvas')
    expect(canvasBefore).toHaveAttribute('data-hidden-clusters', '')

    const valleyPill = screen.getByText(/Valley Bar/).closest('button')!
    expect(valleyPill).toHaveAttribute('aria-pressed', 'true') // visible
    await user.click(valleyPill)

    const valleyPillAfter = screen.getByText(/Valley Bar/).closest('button')!
    expect(valleyPillAfter).toHaveAttribute('aria-pressed', 'false') // hidden
    expect(screen.getByTestId('scene-graph-canvas')).toHaveAttribute(
      'data-hidden-clusters',
      'v_1',
    )
  })

  it('renders nothing when there are zero nodes', async () => {
    const hooks = await import('../hooks/useScenes')
    vi.mocked(hooks.useSceneGraph).mockReturnValueOnce({
      data: { ...mockData, nodes: [], links: [], clusters: [] },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing while loading', async () => {
    const hooks = await import('../hooks/useScenes')
    vi.mocked(hooks.useSceneGraph).mockReturnValueOnce({
      data: undefined,
      isLoading: true,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />,
    )
    expect(container.firstChild).toBeNull()
  })
})
