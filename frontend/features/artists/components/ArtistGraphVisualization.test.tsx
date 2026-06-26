import { describe, it, expect, vi, afterEach } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// PSY-1218: cover the PARENT's hover-grace dismiss orchestration — the 300ms
// dismiss timer, cancel-on-pointer-enter, reschedule-on-leave, and cancel-on-zoom.
// This logic lives in ArtistGraphVisualization and is wired through the canvas's
// onNodeHover/onZoom callbacks, so ArtistNodeTooltip.test.tsx (which renders the
// leaf tooltip in isolation) can't reach it. Without this suite a refactor that
// dropped the timer/cancel wiring would re-introduce the PSY-1218 bug (the link
// becoming unreachable) or a tooltip stranded after zoom, with nothing failing.

// Capture the props the (mocked) ForceGraph2D is rendered with so the test can
// drive onNodeHover / onZoom exactly as the real canvas would.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

// ForceGraph2D loads via next/dynamic (ssr:false). Mock next/dynamic to a
// synchronous stub that records its props — no Suspense/async-import dance and no
// real canvas/three dependency (matches the next/dynamic mock used in TopBar.test).
vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

// With the canvas mocked there is no graph2ScreenCoords and the nodes carry no
// settled d3-force coords, so the real nodeTooltipPlacement returns null for every
// hover and the tooltip would never show. Stub it to place a real node and reject
// null (hover-out), exercising the parent's if/else dismiss branches; keep
// tooltipPlacementStyle real so ArtistNodeTooltip renders exactly as in production.
vi.mock('@/components/graph/nodeTooltip', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/graph/nodeTooltip')>()
  return {
    ...actual,
    nodeTooltipPlacement: (_graph: unknown, _container: unknown, node: unknown) =>
      node ? { x: 12, y: 12, flipX: false, flipY: false } : null,
  }
})

// Next.js Link renders as a plain <a> in jsdom (same shim as ArtistNodeTooltip.test).
vi.mock('next/link', () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}))

import { ArtistGraphVisualization } from './ArtistGraph'

const graphData: ArtistGraph = {
  center: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', city: 'Phoenix', state: 'AZ', upcoming_show_count: 3 },
  nodes: [
    { id: 2, name: 'Frozen Soul', slug: 'frozen-soul', city: 'Fort Worth', state: 'TX', upcoming_show_count: 1 },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'similar', score: 0.85, votes_up: 8, votes_down: 2 },
  ],
  user_votes: {},
}

// The satellite GraphNode the canvas would hand to onNodeHover. Non-center, so the
// rich tooltip (with the "View artist page" link) renders for it.
const satellite = {
  id: 2,
  name: 'Frozen Soul',
  slug: 'frozen-soul',
  city: 'Fort Worth',
  state: 'TX',
  upcoming_show_count: 1,
  isCenter: false,
  val: 4,
}

const renderViz = () =>
  renderWithProviders(
    <ArtistGraphVisualization
      data={graphData}
      activeTypes={new Set(['similar'])}
      containerWidth={1024}
    />
  )

const tooltipLink = () => screen.queryByRole('link', { name: /View artist page/i })

describe('ArtistGraphVisualization — hover-grace tooltip dismissal (PSY-1218)', () => {
  afterEach(() => {
    vi.useRealTimers()
    forceGraphProps = null
  })

  it('shows the tooltip (and its link) when a satellite node is hovered', () => {
    renderViz()
    expect(forceGraphProps).not.toBeNull()
    act(() => forceGraphProps.onNodeHover(satellite))
    expect(tooltipLink()).toBeInTheDocument()
  })

  it('delays dismissal on hover-out, then hides after the grace delay', () => {
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    expect(tooltipLink()).toBeInTheDocument()

    vi.useFakeTimers()
    // Hover-out must NOT vanish the tooltip immediately — the cursor needs time to
    // travel onto it and click the link (the whole point of PSY-1218).
    act(() => forceGraphProps.onNodeHover(null))
    expect(tooltipLink()).toBeInTheDocument()

    // ...and it does dismiss once the grace window elapses.
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).not.toBeInTheDocument()
  })

  it('cancels the pending dismiss when the pointer enters the tooltip', () => {
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))

    vi.useFakeTimers()
    act(() => forceGraphProps.onNodeHover(null)) // arm the dismiss timer
    // Cursor reaches the tooltip → onMouseEnter cancels the timer, so it survives
    // well past the grace window.
    fireEvent.mouseEnter(screen.getByTestId('artist-node-tooltip'))
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).toBeInTheDocument()
  })

  it('reschedules the dismiss when the pointer leaves the tooltip', () => {
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))

    vi.useFakeTimers()
    act(() => forceGraphProps.onNodeHover(null))
    const wrapper = screen.getByTestId('artist-node-tooltip')
    fireEvent.mouseEnter(wrapper) // keep open
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).toBeInTheDocument()

    // Leaving the tooltip re-arms the dismiss, which then fires.
    fireEvent.mouseLeave(wrapper)
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).not.toBeInTheDocument()
  })

  it('dismisses immediately on zoom, with no lingering grace window', () => {
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    expect(tooltipLink()).toBeInTheDocument()

    // Wheel-zoom strands the tooltip at a stale screen position (PSY-1215); onZoom
    // hides it now AND cancels any pending dismiss timer so it can't fire later.
    act(() => forceGraphProps.onZoom())
    expect(tooltipLink()).not.toBeInTheDocument()
  })
})
