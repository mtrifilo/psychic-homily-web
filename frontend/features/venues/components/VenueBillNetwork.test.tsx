import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { VenueBillNetworkResponse } from '../types'

// Mock the useVenueBillNetwork hook before the component imports it.
const mockData: VenueBillNetworkResponse = {
  venue: {
    id: 1,
    slug: 'valley-bar-phoenix-az',
    name: 'Valley Bar',
    city: 'Phoenix',
    state: 'AZ',
    // Backend invariant: artist_count === len(nodes) (4 below).
    artist_count: 4,
    artist_total: 4,
    roster_truncated: false,
    edge_count: 5,
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
    {
      id: 4,
      name: 'Lonely Touring Act',
      slug: 'lonely',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: true,
      at_venue_show_count: 1,
    },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'shared_bills', score: 0.5, is_cross_cluster: false },
    { source_id: 1, target_id: 3, type: 'shared_bills', score: 0.4, is_cross_cluster: false },
    { source_id: 2, target_id: 3, type: 'shared_bills', score: 0.3, is_cross_cluster: false },
  ],
}

vi.mock('../hooks/useVenues', () => ({
  useVenueBillNetwork: vi.fn(() => ({
    data: mockData,
    isLoading: false,
    error: null,
  })),
}))

// jsdom can't render canvas; stub the visualization adapter so we can assert
// container behavior + prop forwarding without touching d3-force.
vi.mock('./VenueBillNetworkAdapter', () => ({
  SceneGraphVisualizationStyleAdapter: ({
    venueName,
    countPhrase,
    height,
  }: {
    venueName: string
    countPhrase: string
    height?: number
  }) => (
    <div
      data-testid="venue-bill-network-canvas"
      data-venue-name={venueName}
      data-count-phrase={countPhrase}
      data-height={height ?? ''}
    >
      Venue Bill Network Canvas
    </div>
  ),
}))

import { VenueBillNetwork } from './VenueBillNetwork'

describe('VenueBillNetwork', () => {
  let ro: ReturnType<typeof installImmediateResizeObserver>

  beforeEach(async () => {
    // Shared ResizeObserver shim (test/mocks/resizeObserver) so we can drive
    // the >=640px graph gate.
    ro = installImmediateResizeObserver(1024)
    // Reset the hook mock to the default mockData so individual tests can
    // override with `.mockReturnValue` without leaking to the next test.
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: mockData,
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
  })

  afterEach(() => {
    ro.restore()
  })

  it('renders the section header and counts', () => {
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    expect(screen.getByText(/Who plays together here/)).toBeInTheDocument()
    expect(screen.getByText(/4 artists/)).toBeInTheDocument()
    expect(screen.getByText(/5 co-bills/)).toBeInTheDocument()
    expect(screen.getByText(/1 unconnected/)).toBeInTheDocument()
  })

  it('discloses the top-N-of-M cap when the roster is truncated (PSY-1476)', async () => {
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: {
        ...mockData,
        // 4 nodes shown, but 312 distinct in-window artists before the cap.
        venue: { ...mockData.venue, artist_total: 312, roster_truncated: true },
      },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    expect(screen.getByText(/Top 4 of 312 artists/)).toBeInTheDocument()
    // The tail (co-bills / unconnected) is unchanged.
    expect(screen.getByText(/5 co-bills/)).toBeInTheDocument()
    // The same phrase (lowercase, mid-sentence) is threaded to the canvas
    // aria-label so header and assistive tech can't diverge.
    expect(screen.getByTestId('venue-bill-network-canvas')).toHaveAttribute(
      'data-count-phrase',
      'top 4 of 312 artists',
    )
  })

  it('degrades to the plain count when truncated but the total is not larger (PSY-1476)', async () => {
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: {
        ...mockData,
        // Stale/skewed payload: flag set but total ≤ shown — no "top 4 of 4".
        venue: { ...mockData.venue, artist_total: 4, roster_truncated: true },
      },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    expect(screen.getByText(/4 artists/)).toBeInTheDocument()
    expect(screen.queryByText(/Top 4 of/)).not.toBeInTheDocument()
  })

  it('hides the canvas below the 640px breakpoint', () => {
    ro.setWidth(500)
    const { container } = renderWithProviders(
      <VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />,
    )
    // Even when the venue is non-sparse, the canvas is gated; on this
    // dataset (4 artists, 25 shows) it's non-sparse but mobile-gated.
    expect(screen.queryByTestId('venue-bill-network-canvas')).not.toBeInTheDocument()
    // PSY-1472: non-sparse mobile shows the teaser card — connectedness pitch
    // + a link-out that scrolls to the venue's show list on this page.
    expect(
      screen.getByText(/Who plays Valley Bar together, mapped by shared bills/i),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Browse shows at Valley Bar/i }),
    ).toHaveAttribute('href', '#venue-shows')
    expect(container).toBeTruthy()
  })

  it('renders canvas + window filter at desktop width', () => {
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    expect(screen.getByTestId('venue-bill-network-canvas')).toBeInTheDocument()
    expect(screen.getByText(/All-time/)).toBeInTheDocument()
    expect(screen.getByText(/Last 12 months/)).toBeInTheDocument()
    expect(screen.getByText(/By year/)).toBeInTheDocument()
  })

  it('all-time is the default selected window', () => {
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    const allTime = screen.getByRole('button', { name: /^All-time$/ })
    expect(allTime).toHaveAttribute('aria-pressed', 'true')
    const last12m = screen.getByRole('button', { name: /^Last 12 months$/ })
    expect(last12m).toHaveAttribute('aria-pressed', 'false')
  })

  it('switching to "By year" reveals the year picker', async () => {
    const user = userEvent.setup()
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)

    expect(screen.queryByRole('combobox', { name: /select year/i })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^By year$/ }))

    expect(screen.getByRole('combobox', { name: /select year/i })).toBeInTheDocument()
  })

  it('passes the venue name to the visualization adapter', () => {
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    const canvas = screen.getByTestId('venue-bill-network-canvas')
    expect(canvas).toHaveAttribute('data-venue-name', 'Valley Bar')
  })

  it('renders the empty-state message for sparse venues', async () => {
    const hooks = await import('../hooks/useVenues')
    // Use mockReturnValue (sticky for the test) — React may invoke the
    // hook twice in dev/StrictMode and `mockReturnValueOnce` would fall
    // back to the default mockData on the second call.
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: {
        ...mockData,
        venue: { ...mockData.venue, show_count: 5 }, // below MIN_GRAPH_SHOWS=10
      },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Tiny Bar" />)
    expect(
      screen.getByText(/Not enough booked-together activity yet/i),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('venue-bill-network-canvas')).not.toBeInTheDocument()
  })

  it('treats <3 connected artists as sparse even with enough shows', async () => {
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: {
        ...mockData,
        venue: { ...mockData.venue, show_count: 25, artist_count: 5, edge_count: 0 },
        // All nodes set to isolate — connectedCount=0, below MIN_GRAPH_NODES=3
        nodes: mockData.nodes.map(n => ({ ...n, is_isolate: true })),
        links: [],
      },
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Sparse Bar" />)
    expect(
      screen.getByText(/Not enough booked-together activity yet/i),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('venue-bill-network-canvas')).not.toBeInTheDocument()
  })

  it('does not infinite-loop on mobile widths (React #185 regression)', async () => {
    // Regression for the venue-page mobile crash. At < 640px the section used
    // to `return null`, which unmounted the useContainerWidth ref node; the
    // hook's cleanup resets the measured width to null on unmount, so the node
    // remounted → remeasured (< 640) → returned null → unmounted … forever
    // ("Maximum update depth exceeded", React #185). The measuring wrapper
    // must stay mounted; only its content is gated.
    //
    // PSY-1777 raised the stakes: the measured width now also gates the FETCH,
    // so an unmounted ref would additionally deadlock the query (no
    // measurement → never enabled → no data). Mobile therefore has NO payload,
    // which is what this fixture models — sparseness is unknowable here, so
    // the teaser is the correct state rather than the old blank section.
    ro.setWidth(400)
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      // The disabled-query shape react-query reports at mobile widths:
      // pending forever, never fetching, so `isLoading` is false.
      data: undefined,
      isLoading: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)

    // Rendering must not throw — the old code threw React #185 here.
    expect(() =>
      renderWithProviders(
        <VenueBillNetwork venueIdOrSlug={1} venueName="Sparse Mobile Bar" />,
      ),
    ).not.toThrow()

    // No canvas, and no infinite skeleton: the teaser is the terminal mobile
    // state even with no payload to describe.
    expect(screen.queryByTestId('venue-bill-network-canvas')).not.toBeInTheDocument()
    expect(
      screen.getByText(/mapped by shared bills here\. Needs a larger screen/i),
    ).toBeInTheDocument()
  })

  // ------------------------------------------------------------------
  // PSY-1777: the viewport gate must precede the fetch, not follow it.
  // ------------------------------------------------------------------
  describe('viewport-gated fetching (PSY-1777)', () => {
    it('DISABLES the bill-network query below the 640px breakpoint', async () => {
      ro.setWidth(390)
      const hooks = await import('../hooks/useVenues')
      vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
        data: undefined,
        isLoading: false,
        error: null,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
      } as any)

      renderWithProviders(
        <VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />,
      )

      // Every call the component made must have been gated off — asserting on
      // the LAST call alone would pass even if an early render had fetched.
      const calls = vi.mocked(hooks.useVenueBillNetwork).mock.calls
      expect(calls.length).toBeGreaterThan(0)
      for (const [options] of calls) {
        expect(options.enabled).toBe(false)
      }

      // ...and the visitor still gets the teaser, not a stuck skeleton.
      expect(
        screen.getByText(/mapped by shared bills here\. Needs a larger screen/i),
      ).toBeInTheDocument()
    })

    it('ENABLES the bill-network query once measured at desktop width', async () => {
      ro.setWidth(1280)
      const hooks = await import('../hooks/useVenues')

      renderWithProviders(
        <VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />,
      )

      // The settled render must be enabled (the pre-measurement first render
      // is legitimately disabled — containerWidth is null before the
      // ResizeObserver reports).
      const calls = vi.mocked(hooks.useVenueBillNetwork).mock.calls
      expect(calls[calls.length - 1][0].enabled).toBe(true)
      expect(screen.getByTestId('venue-bill-network-canvas')).toBeInTheDocument()
    })
  })

  it('renders the header + a height-reserving skeleton (not null) while loading (PSY-1446)', async () => {
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    const { container } = renderWithProviders(
      <VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />,
    )
    expect(screen.getByText(/Who plays together here/)).toBeInTheDocument()
    expect(container.querySelector('.animate-pulse')).not.toBeNull()
    expect(screen.queryByTestId('venue-bill-network-canvas')).not.toBeInTheDocument()
  })

  it('shows a visible error card + the window filter when the fetch settles in error (PSY-1446)', async () => {
    const hooks = await import('../hooks/useVenues')
    vi.mocked(hooks.useVenueBillNetwork).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Internal Server Error'),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any)
    renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
    expect(screen.getByText(/Who plays together here/)).toBeInTheDocument()
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(/couldn't load/i)
    // The window filter stays rendered — it's the user's path back to a
    // window that worked (SceneGraph's stranding rationale).
    expect(screen.getByRole('button', { name: /^All-time$/ })).toBeInTheDocument()
    expect(screen.queryByTestId('venue-bill-network-canvas')).not.toBeInTheDocument()
  })

  describe('fullscreen overlay', () => {
    it('renders the Expand button at desktop width when graph is available', () => {
      renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
      expect(
        screen.getByRole('button', {
          name: /expand venue bill network to fullscreen/i,
        }),
      ).toBeInTheDocument()
    })

    it('opens the overlay when Expand is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)

      expect(screen.queryByTestId('venue-bill-network-overlay')).not.toBeInTheDocument()

      await user.click(
        screen.getByRole('button', {
          name: /expand venue bill network to fullscreen/i,
        }),
      )

      const overlay = screen.getByTestId('venue-bill-network-overlay')
      expect(overlay).toBeInTheDocument()
      expect(overlay).toHaveAttribute('role', 'dialog')
      expect(overlay).toHaveAttribute('aria-modal', 'true')
    })

    it('closes the overlay when Esc is pressed', async () => {
      const user = userEvent.setup()
      renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)

      await user.click(
        screen.getByRole('button', {
          name: /expand venue bill network to fullscreen/i,
        }),
      )
      expect(screen.getByTestId('venue-bill-network-overlay')).toBeInTheDocument()

      await user.keyboard('{Escape}')
      expect(screen.queryByTestId('venue-bill-network-overlay')).not.toBeInTheDocument()
    })

    it('keeps the window filter interactive inside the overlay', async () => {
      const user = userEvent.setup()
      renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)

      await user.click(
        screen.getByRole('button', {
          name: /expand venue bill network to fullscreen/i,
        }),
      )
      const overlay = screen.getByTestId('venue-bill-network-overlay')

      // The window filter is rendered inside the overlay too. Switching
      // shouldn't dismiss the overlay.
      const last12m = within(overlay).getByRole('button', { name: /^Last 12 months$/ })
      await user.click(last12m)

      expect(screen.getByTestId('venue-bill-network-overlay')).toBeInTheDocument()
    })

    it('locks body scroll while open and restores on close', async () => {
      const user = userEvent.setup()
      document.body.style.overflow = 'auto'

      renderWithProviders(<VenueBillNetwork venueIdOrSlug={1} venueName="Valley Bar" />)
      expect(document.body.style.overflow).toBe('auto')

      await user.click(
        screen.getByRole('button', {
          name: /expand venue bill network to fullscreen/i,
        }),
      )
      expect(document.body.style.overflow).toBe('hidden')

      await user.keyboard('{Escape}')
      expect(document.body.style.overflow).toBe('auto')

      document.body.style.overflow = ''
    })
  })
})
