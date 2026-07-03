import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { SceneGraphResponse } from '../types'

// Mock the useSceneGraph hook before the SceneGraph component imports it.
const mockData: SceneGraphResponse = {
  scene: {
    slug: 'phoenix-az',
    city: 'Phoenix',
    state: 'AZ',
    // artist_count mirrors nodes.length below — the backend guarantees the
    // two are equal (ArtistCount: len(rows)), and the header renders this
    // contract field via sceneArtistCountPhrase (PSY-1296).
    artist_count: 4,
    edge_count: 4,
    metro_roster_total: 4,
    roster_truncated: false,
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
// Forward `height` as a data attribute so the overlay sizing test can verify
// the prop reaches the visualization.
vi.mock('./SceneGraphVisualization', () => ({
  SceneGraphVisualization: ({
    hiddenClusterIDs,
    height,
  }: {
    hiddenClusterIDs: Set<string>
    height?: number
  }) => (
    <div
      data-testid="scene-graph-canvas"
      data-hidden-clusters={Array.from(hiddenClusterIDs).sort().join(',')}
      data-height={height ?? ''}
    >
      Scene Graph Canvas
    </div>
  ),
}))

import { SceneGraph } from './SceneGraph'
import { useSceneGraph } from '../hooks/useScenes'

describe('SceneGraph', () => {
  // Shared immediate ResizeObserver shim so we can drive the >= 640px graph
  // gate (test/mocks/resizeObserver.ts).
  let resizeObserver: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(() => {
    resizeObserver = installImmediateResizeObserver()
    // Re-seed the default result every test so an override in one test can't
    // leak into the next (no hidden test-order coupling).
    vi.mocked(useSceneGraph).mockImplementation(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      () => ({ data: mockData, isLoading: false, error: null }) as any,
    )
  })

  afterEach(() => {
    resizeObserver.restore()
  })

  it('renders the section header and counts', () => {
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
    expect(screen.getByText('Scene graph')).toBeInTheDocument()
    expect(screen.getByText(/4 artists/)).toBeInTheDocument()
    expect(screen.getByText(/4 connections/)).toBeInTheDocument()
    expect(screen.getByText(/1 unconnected/)).toBeInTheDocument()
    // Untruncated roster → plain count, no "top N of M" hint (PSY-1296).
    expect(screen.queryByText(/top \d+ of \d+/i)).not.toBeInTheDocument()
  })

  // PSY-1296: a graph capped by PSY-1277's top-N roster limit must say so —
  // a bare count on a capped graph reads as the whole scene.
  it('shows the "top N of M" hint when the roster was truncated', () => {
    // NOT mockReturnValueOnce: the container-width measurement re-renders the
    // component, so the hook is called more than once per mount and the Once
    // value would only cover the first (pre-measurement) render. beforeEach
    // restores the default implementation for the next test.
    vi.mocked(useSceneGraph).mockImplementation(
      () =>
        ({
          data: {
            ...mockData,
            scene: {
              ...mockData.scene,
              metro_roster_total: 90,
              roster_truncated: true,
            },
          },
          isLoading: false,
          error: null,
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
        }) as any,
    )
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

    // Sentence-cased in the visual header (the shared phrase is lowercase for
    // mid-sentence aria-label use).
    expect(screen.getByText(/Top 4 of 90 artists/)).toBeInTheDocument()
  })

  it('keeps the truncation hint in the fullscreen overlay header (shared element)', async () => {
    vi.mocked(useSceneGraph).mockImplementation(
      () =>
        ({
          data: {
            ...mockData,
            scene: {
              ...mockData.scene,
              metro_roster_total: 90,
              roster_truncated: true,
            },
          },
          isLoading: false,
          error: null,
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
        }) as any,
    )
    const user = userEvent.setup()
    renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

    await user.click(
      screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
    )
    const overlay = screen.getByTestId('scene-graph-overlay')
    expect(within(overlay).getByText(/Top 4 of 90 artists/)).toBeInTheDocument()
  })

  it('hides the canvas below the 640px breakpoint', () => {
    resizeObserver.setWidth(500)
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
    // PSY-1296 caption: the roster is BASED-IN artists, not played-here.
    expect(
      screen.getByText(/based in the Phoenix, AZ scene, ranked by their approved shows here/),
    ).toBeInTheDocument()
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

  describe('fullscreen overlay (PSY-517)', () => {
    it('renders the Expand button when graph is available at desktop width', () => {
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
      expect(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      ).toBeInTheDocument()
    })

    it('does NOT render the Expand button below the 640px breakpoint (mobile gate inherited)', () => {
      resizeObserver.setWidth(500)
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
      // Mobile gate: graphAvailable === false → Expand button isn't rendered.
      expect(
        screen.queryByRole('button', { name: /expand scene graph to fullscreen/i }),
      ).not.toBeInTheDocument()
    })

    it('opens the overlay when Expand is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      expect(screen.queryByTestId('scene-graph-overlay')).not.toBeInTheDocument()

      await user.click(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      )

      const overlay = screen.getByTestId('scene-graph-overlay')
      expect(overlay).toBeInTheDocument()
      expect(overlay).toHaveAttribute('role', 'dialog')
      expect(overlay).toHaveAttribute('aria-modal', 'true')
      // The Exit button replaces Expand in the header.
      expect(
        screen.getByRole('button', { name: /exit fullscreen scene graph/i }),
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /expand scene graph to fullscreen/i }),
      ).not.toBeInTheDocument()
    })

    it('closes the overlay when the Exit button is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      await user.click(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      )
      expect(screen.getByTestId('scene-graph-overlay')).toBeInTheDocument()

      await user.click(
        screen.getByRole('button', { name: /exit fullscreen scene graph/i }),
      )
      expect(screen.queryByTestId('scene-graph-overlay')).not.toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      ).toBeInTheDocument()
    })

    it('closes the overlay when Esc is pressed', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      await user.click(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      )
      expect(screen.getByTestId('scene-graph-overlay')).toBeInTheDocument()

      await user.keyboard('{Escape}')

      expect(screen.queryByTestId('scene-graph-overlay')).not.toBeInTheDocument()
    })

    it('locks body scroll while open and restores the previous value on close', async () => {
      const user = userEvent.setup()
      // Seed an inline overflow value so we can verify the previous-value
      // restore (not a blind reset to '').
      document.body.style.overflow = 'auto'

      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)
      expect(document.body.style.overflow).toBe('auto')

      await user.click(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      )
      expect(document.body.style.overflow).toBe('hidden')

      await user.keyboard('{Escape}')
      expect(document.body.style.overflow).toBe('auto')

      // Cleanup so the next test isn't affected.
      document.body.style.overflow = ''
    })

    it('keeps cluster pills interactive inside the overlay', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      await user.click(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      )
      const overlay = screen.getByTestId('scene-graph-overlay')

      // Pill inside the overlay is rendered + clickable + reflects state on the
      // overlay's canvas (which receives the same hiddenClusterIDs set).
      const valleyPill = within(overlay).getByText(/Valley Bar/).closest('button')!
      expect(valleyPill).toHaveAttribute('aria-pressed', 'true')

      await user.click(valleyPill)

      const valleyPillAfter = within(overlay).getByText(/Valley Bar/).closest('button')!
      expect(valleyPillAfter).toHaveAttribute('aria-pressed', 'false')

      const overlayCanvas = within(overlay).getByTestId('scene-graph-canvas')
      expect(overlayCanvas).toHaveAttribute('data-hidden-clusters', 'v_1')
    })
  })

  describe('cluster-by toggle (PSY-1320)', () => {
    it('defaults to venue mode (amended decision pending PSY-1323)', () => {
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      expect(screen.getByRole('button', { name: 'Venue' })).toHaveAttribute(
        'aria-pressed',
        'true',
      )
      expect(screen.getByRole('button', { name: 'Community' })).toHaveAttribute(
        'aria-pressed',
        'false',
      )
      expect(screen.getByText(/most-frequent venue/)).toBeInTheDocument()
    })

    it('switches the hook to community mode and updates the caption', async () => {
      const user = userEvent.setup()
      const hooks = await import('../hooks/useScenes')
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      await user.click(screen.getByRole('button', { name: 'Community' }))

      expect(screen.getByRole('button', { name: 'Community' })).toHaveAttribute(
        'aria-pressed',
        'true',
      )
      expect(screen.getByRole('button', { name: 'Venue' })).toHaveAttribute(
        'aria-pressed',
        'false',
      )
      // The hook re-renders with the new mode — the query key change is what
      // triggers the refetch (covered by useSceneGraph.test.tsx).
      const lastCall = vi.mocked(hooks.useSceneGraph).mock.calls.at(-1)![0]
      expect(lastCall.clusterBy).toBe('community')
      expect(screen.getByText(/similarity community/)).toBeInTheDocument()
    })

    it('resets hidden clusters when the mode switches (IDs are mode-scoped)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      await user.click(screen.getByText(/Valley Bar/).closest('button')!)
      expect(screen.getByTestId('scene-graph-canvas')).toHaveAttribute(
        'data-hidden-clusters',
        'v_1',
      )

      await user.click(screen.getByRole('button', { name: 'Community' }))
      expect(screen.getByTestId('scene-graph-canvas')).toHaveAttribute(
        'data-hidden-clusters',
        '',
      )
    })

    it('keeps the toggle rendered when a mode fetch settles in error', async () => {
      const hooks = await import('../hooks/useScenes')
      vi.mocked(hooks.useSceneGraph).mockReturnValueOnce({
        data: undefined,
        isLoading: false,
        isError: true,
        isPlaceholderData: false,
        error: new Error('boom'),
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
      } as any)
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      // The section must NOT unmount: the toggle is the only path back to the
      // mode that worked.
      expect(screen.getByText('Scene graph')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Venue' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Community' })).toBeInTheDocument()
      expect(screen.getByText(/couldn't load/i)).toBeInTheDocument()
      expect(screen.queryByTestId('scene-graph-canvas')).not.toBeInTheDocument()
    })

    it('dims and inert-blocks the stale clusters while the mode switch is in flight', async () => {
      const hooks = await import('../hooks/useScenes')
      const mocked = vi.mocked(hooks.useSceneGraph)
      // mockImplementation (not ...Once): the width-measurement setState
      // re-renders the component, so the hook is called more than once.
      mocked.mockImplementation(
        () =>
          ({
            data: mockData,
            isLoading: false,
            isError: false,
            isPlaceholderData: true,
            error: null,
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
          }) as any,
      )
      try {
        renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

        const busyRegion = screen
          .getByTestId('scene-graph-canvas')
          .closest('[aria-busy="true"]')
        expect(busyRegion).not.toBeNull()
        expect(busyRegion!.className).toContain('pointer-events-none')
        // The mode toggle itself stays outside the blocked region.
        expect(
          screen.getByRole('button', { name: 'Venue' }).closest('[aria-busy="true"]'),
        ).toBeNull()
      } finally {
        mocked.mockImplementation(
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          () => ({ data: mockData, isLoading: false, error: null }) as any,
        )
      }
    })

    it('renders the toggle inside the fullscreen overlay', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SceneGraph slug="phoenix-az" city="Phoenix" state="AZ" />)

      await user.click(
        screen.getByRole('button', { name: /expand scene graph to fullscreen/i }),
      )
      const overlay = screen.getByTestId('scene-graph-overlay')

      const communityPill = within(overlay).getByRole('button', { name: 'Community' })
      await user.click(communityPill)
      expect(
        within(overlay).getByRole('button', { name: 'Community' }),
      ).toHaveAttribute('aria-pressed', 'true')
    })
  })
})
