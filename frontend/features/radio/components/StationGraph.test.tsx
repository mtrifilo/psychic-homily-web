import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { RadioStationGraphResponse } from '../types'

// Mock the useStationGraph hook before StationGraph imports it.
const mockData: RadioStationGraphResponse = {
  station: {
    id: 1,
    slug: 'kexp',
    name: 'KEXP',
    artist_count: 12,
    // Distinct from nodes.length (4) so a nodeCount/edgeCount variable swap
    // in the header can't slip past the count assertions.
    edge_count: 5,
    window: 'last_12m',
  },
  clusters: [
    { id: 'rs_1', label: 'The Morning Show', size: 6, color_index: 0 },
    { id: 'rs_2', label: 'Wo Pop', size: 6, color_index: 1 },
  ],
  nodes: [
    {
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'rs_1',
      is_isolate: false,
      play_count: 12,
    },
    {
      id: 2,
      name: 'Sundressed',
      slug: 'sundressed',
      upcoming_show_count: 1,
      cluster_id: 'rs_1',
      is_isolate: false,
      play_count: 9,
    },
    {
      id: 3,
      name: 'Numb Bats',
      slug: 'numb-bats',
      upcoming_show_count: 0,
      cluster_id: 'rs_2',
      is_isolate: false,
      play_count: 7,
    },
    {
      id: 4,
      name: 'Lonely Lounge',
      slug: 'lonely-lounge',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: true,
      play_count: 2,
    },
  ],
  links: [
    {
      source_id: 1,
      target_id: 2,
      type: 'radio_cooccurrence',
      score: 0.5,
      is_cross_cluster: false,
    },
    {
      source_id: 1,
      target_id: 3,
      type: 'radio_cooccurrence',
      score: 0.3,
      is_cross_cluster: true,
    },
    {
      source_id: 2,
      target_id: 3,
      type: 'radio_cooccurrence',
      score: 0.2,
      is_cross_cluster: true,
    },
    {
      source_id: 1,
      target_id: 4,
      type: 'radio_cooccurrence',
      score: 0.2,
      is_cross_cluster: true,
    },
  ],
}

vi.mock('../hooks/useStationGraph', () => ({
  useStationGraph: vi.fn(() => ({
    data: mockData,
    isLoading: false,
    error: null,
  })),
}))

// Canvas can't render in jsdom. Stub the visualization so we can assert
// toggling; forward height so the overlay sizing path is observable.
vi.mock('./StationGraphVisualization', () => ({
  StationGraphVisualization: ({
    hiddenClusterIDs,
    height,
  }: {
    hiddenClusterIDs: Set<string>
    height?: number
  }) => (
    <div
      data-testid="station-graph-canvas"
      data-hidden-clusters={Array.from(hiddenClusterIDs).sort().join(',')}
      data-height={height ?? ''}
    >
      Station Graph Canvas
    </div>
  ),
}))

import { StationGraph } from './StationGraph'

// Shared immediate ResizeObserver shim (PSY-1305): drives the >= 640px graph
// gate on mount and fireResize() simulates the viewport crossing the
// breakpoint AFTER mount (the overlay auto-close path).
let ro: ReturnType<typeof installImmediateResizeObserver>

function setMockContainerWidth(width: number) {
  ro.setWidth(width)
}

function fireResize(width: number) {
  ro.fireResize(width)
}

describe('StationGraph', () => {
  beforeEach(async () => {
    ro = installImmediateResizeObserver(1024)
    // Reset the hook mock to the default mockData so tests that override with
    // sticky `.mockReturnValue` (the error-card test) can't leak forward.
    const hooks = await import('../hooks/useStationGraph')
    vi.mocked(hooks.useStationGraph).mockImplementation(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      () => ({ data: mockData, isLoading: false, error: null }) as any,
    )
  })

  afterEach(() => {
    ro.restore()
  })

  it('renders the section header and counts', () => {
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
    expect(screen.getByText('Airplay graph')).toBeInTheDocument()
    expect(screen.getByText(/4 artists/)).toBeInTheDocument()
    expect(screen.getByText(/5 connections/)).toBeInTheDocument()
    expect(screen.getByText(/1 unconnected/)).toBeInTheDocument()
  })

  it('captions the graph with the station name and default window', () => {
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
    expect(
      screen.getByText(/Artists most played on KEXP over the last 12 months/),
    ).toBeInTheDocument()
  })

  it('hides the canvas below the 640px breakpoint and shows the teaser card (PSY-1446)', () => {
    setMockContainerWidth(500)
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
    expect(screen.queryByTestId('station-graph-canvas')).not.toBeInTheDocument()
    expect(screen.queryByText(/The Morning Show \(6\)/)).not.toBeInTheDocument()
    // PSY-1472: the teaser card carries the connectedness pitch + a link-out
    // that scrolls to the station's playlists feed on this page.
    expect(
      screen.getByText(/KEXP.s airplay as a map/i),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /See recent playlists/i }),
    ).toHaveAttribute('href', '#recent-playlists')
  })

  it('renders canvas + cluster legend at desktop width', () => {
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
    expect(screen.getByTestId('station-graph-canvas')).toBeInTheDocument()
    expect(screen.getByText(/The Morning Show \(6\)/)).toBeInTheDocument()
    expect(screen.getByText(/Wo Pop \(6\)/)).toBeInTheDocument()
  })

  it('toggles cluster visibility when a legend pill is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)

    const canvasBefore = screen.getByTestId('station-graph-canvas')
    expect(canvasBefore).toHaveAttribute('data-hidden-clusters', '')

    const morningPill = screen.getByText(/The Morning Show/).closest('button')!
    expect(morningPill).toHaveAttribute('aria-pressed', 'true')
    await user.click(morningPill)

    const morningPillAfter = screen.getByText(/The Morning Show/).closest('button')!
    expect(morningPillAfter).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByTestId('station-graph-canvas')).toHaveAttribute(
      'data-hidden-clusters',
      'rs_1',
    )
  })

  it('renders the id="graph" deep-link anchor', () => {
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
    const anchor = document.getElementById('graph')
    expect(anchor).toBeInTheDocument()
    expect(anchor).toContainElement(screen.getByText('Airplay graph'))
  })

  it('renders nothing below MIN_GRAPH_NODES (no dangling header)', async () => {
    const hooks = await import('../hooks/useStationGraph')
    vi.mocked(hooks.useStationGraph).mockReturnValueOnce({
      data: {
        ...mockData,
        station: { ...mockData.station, artist_count: 2, edge_count: 1 },
        nodes: mockData.nodes.slice(0, 2),
        links: mockData.links.slice(0, 1),
        clusters: mockData.clusters.slice(0, 1),
      },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <StationGraph slug="kexp" stationName="KEXP" />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when there are zero nodes', async () => {
    const hooks = await import('../hooks/useStationGraph')
    vi.mocked(hooks.useStationGraph).mockReturnValueOnce({
      data: { ...mockData, nodes: [], links: [], clusters: [] },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <StationGraph slug="kexp" stationName="KEXP" />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders the header + a height-reserving skeleton (not null) while loading (PSY-1446)', async () => {
    const hooks = await import('../hooks/useStationGraph')
    vi.mocked(hooks.useStationGraph).mockReturnValueOnce({
      data: undefined,
      isLoading: true,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <StationGraph slug="kexp" stationName="KEXP" />,
    )
    expect(screen.getByText('Airplay graph')).toBeInTheDocument()
    expect(container.querySelector('.animate-pulse')).not.toBeNull()
    expect(screen.queryByTestId('station-graph-canvas')).not.toBeInTheDocument()
  })

  it('shows a visible error card when the graph fetch settles in error (PSY-1446)', async () => {
    // Regression: StationGraph previously had NO error branch — a settled
    // API failure (e.g. a 500) rendered nothing silently, indistinguishable
    // from a sparse station.
    const hooks = await import('../hooks/useStationGraph')
    vi.mocked(hooks.useStationGraph).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Internal Server Error'),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
    expect(screen.getByText('Airplay graph')).toBeInTheDocument()
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(/couldn't load/i)
    expect(screen.queryByTestId('station-graph-canvas')).not.toBeInTheDocument()
  })

  describe('fullscreen overlay', () => {
    it('renders the Expand button when the graph is available at desktop width', () => {
      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
      expect(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      ).toBeInTheDocument()
    })

    it('does NOT render the Expand button below the 640px breakpoint', () => {
      setMockContainerWidth(500)
      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
      expect(
        screen.queryByRole('button', { name: /expand airplay graph to fullscreen/i }),
      ).not.toBeInTheDocument()
    })

    it('opens the overlay when Expand is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)

      expect(screen.queryByTestId('station-graph-overlay')).not.toBeInTheDocument()

      await user.click(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      )

      const overlay = screen.getByTestId('station-graph-overlay')
      expect(overlay).toBeInTheDocument()
      expect(overlay).toHaveAttribute('role', 'dialog')
      expect(overlay).toHaveAttribute('aria-modal', 'true')
      expect(
        screen.getByRole('button', { name: /exit fullscreen airplay graph/i }),
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /expand airplay graph to fullscreen/i }),
      ).not.toBeInTheDocument()
    })

    it('closes the overlay when the Exit button is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)

      await user.click(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      )
      expect(screen.getByTestId('station-graph-overlay')).toBeInTheDocument()

      await user.click(
        screen.getByRole('button', { name: /exit fullscreen airplay graph/i }),
      )
      expect(screen.queryByTestId('station-graph-overlay')).not.toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      ).toBeInTheDocument()
    })

    it('closes the overlay when Esc is pressed', async () => {
      const user = userEvent.setup()
      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)

      await user.click(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      )
      expect(screen.getByTestId('station-graph-overlay')).toBeInTheDocument()

      await user.keyboard('{Escape}')

      expect(screen.queryByTestId('station-graph-overlay')).not.toBeInTheDocument()
    })

    it('locks body scroll while open and restores the previous value on close', async () => {
      const user = userEvent.setup()
      document.body.style.overflow = 'auto'

      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
      expect(document.body.style.overflow).toBe('auto')

      await user.click(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      )
      expect(document.body.style.overflow).toBe('hidden')

      await user.keyboard('{Escape}')
      expect(document.body.style.overflow).toBe('auto')

      document.body.style.overflow = ''
    })

    it('auto-closes (and unlocks scroll) when the viewport drops below the breakpoint', async () => {
      const user = userEvent.setup()
      document.body.style.overflow = 'auto'

      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)
      await user.click(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      )
      expect(screen.getByTestId('station-graph-overlay')).toBeInTheDocument()
      expect(document.body.style.overflow).toBe('hidden')

      // Shrink the observed container below 640px while the overlay is open.
      const { act } = await import('@testing-library/react')
      act(() => fireResize(500))

      // Overlay is gone AND isFullscreen was reset: scroll restored, and the
      // inline section is no longer inert (it would be if isFullscreen were
      // still true with the overlay unmounted).
      expect(screen.queryByTestId('station-graph-overlay')).not.toBeInTheDocument()
      expect(document.body.style.overflow).toBe('auto')

      document.body.style.overflow = ''
    })

    it('keeps cluster pills interactive inside the overlay', async () => {
      const user = userEvent.setup()
      renderWithProviders(<StationGraph slug="kexp" stationName="KEXP" />)

      await user.click(
        screen.getByRole('button', { name: /expand airplay graph to fullscreen/i }),
      )
      const overlay = screen.getByTestId('station-graph-overlay')

      const morningPill = within(overlay).getByText(/The Morning Show/).closest('button')!
      expect(morningPill).toHaveAttribute('aria-pressed', 'true')

      await user.click(morningPill)

      const morningPillAfter = within(overlay)
        .getByText(/The Morning Show/)
        .closest('button')!
      expect(morningPillAfter).toHaveAttribute('aria-pressed', 'false')

      const overlayCanvas = within(overlay).getByTestId('station-graph-canvas')
      expect(overlayCanvas).toHaveAttribute('data-hidden-clusters', 'rs_1')
    })
  })
})
