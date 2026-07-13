import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
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

// A second center for re-center tests — a different center.id triggers the
// render-phase hover reset (the real re-center path, incl. browser back/forward).
const graphData2: ArtistGraph = {
  ...graphData,
  center: { id: 99, name: 'Trve', slug: 'trve', city: 'Tempe', state: 'AZ', upcoming_show_count: 0 },
}

// The center node the canvas would hand to onNodeHover. isCenter → no rich tooltip,
// so the hover handler must treat it like hover-out (crossing it must NOT kill grace).
const centerNode = { ...satellite, id: 1, name: 'Gatecreeper', slug: 'gatecreeper', isCenter: true, val: 8 }

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
  // Fake timers from the start (before render) so the dismiss-grace setTimeout is
  // controllable in every test and the boundary doesn't depend on what gets scheduled
  // between render and a mid-test switch (PSY-1218 review). Safe because the mocked
  // ForceGraph2D forwards no ref, so the component's render effects schedule no timers.
  beforeEach(() => vi.useFakeTimers())
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

  it('treats hovering the CENTER node like hover-out — preserves the grace window', () => {
    // Crossing the center on the way to the tooltip must NOT cancel+hide instantly
    // (the center has no rich tooltip); without the !isCenter gate it would
    // cancelDismiss + setHoveredNode(center) and the render guard would blank the
    // tooltip on the spot, killing the grace window (PSY-1218 review).
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    expect(tooltipLink()).toBeInTheDocument()
    act(() => forceGraphProps.onNodeHover(centerNode))
    expect(tooltipLink()).toBeInTheDocument() // still here — grace preserved, not instant-killed

    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).not.toBeInTheDocument() // and it does dismiss once grace elapses
  })

  it('dismisses immediately on re-center (data.center.id change), with no stale tooltip', () => {
    // AC #3 no-regression counterpart to the zoom test: re-center must hide the
    // tooltip at once, and a dismiss timer pending at re-center must not resurrect it.
    const { rerender } = renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    expect(tooltipLink()).toBeInTheDocument()
    rerender(
      <ArtistGraphVisualization data={graphData2} activeTypes={new Set(['similar'])} containerWidth={1024} />,
    )
    expect(tooltipLink()).not.toBeInTheDocument() // gone synchronously on re-center
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).not.toBeInTheDocument() // a pending timer can't bring it back
  })

  it('does not strand the next tooltip after the tooltip unmounts under the pointer', () => {
    // Regression for the PSY-1218 adversarial-review HIGH: entering the tooltip sets
    // overTooltipRef=true, but React fires no onMouseLeave when a re-center unmounts it
    // out from under the cursor. Without the hoveredNode-sync reset the ref sticks true
    // and wedges the dismiss gate, stranding the NEXT tooltip on a plain hover-out.
    const { rerender } = renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    fireEvent.mouseEnter(screen.getByTestId('artist-node-tooltip')) // overTooltipRef = true

    // Re-center unmounts the tooltip with the pointer still "over" it (no mouseLeave).
    rerender(
      <ArtistGraphVisualization data={graphData2} activeTypes={new Set(['similar'])} containerWidth={1024} />,
    )
    // New graph: hover a node, then move OUT to empty canvas (never onto the tooltip).
    act(() => forceGraphProps.onNodeHover(satellite))
    expect(tooltipLink()).toBeInTheDocument()
    act(() => forceGraphProps.onNodeHover(null))
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).not.toBeInTheDocument() // dismissed, NOT stranded
  })

  it('keeps the tooltip open when the canvas fires hover-out AFTER the pointer enters it', () => {
    // The actual overTooltipRef race the gate defends (PSY-1218): the cursor reaches
    // the tooltip (synchronous onMouseEnter) and only THEN does the canvas fire
    // onNodeHover(null) once on hover-out — a frame later, since canvas hover runs in a
    // requestAnimationFrame loop — re-arming a dismiss. The gate must bail because the
    // pointer is over the tooltip — without it the link would vanish under the cursor.
    // (The existing "cancels on enter" test arms BEFORE entering, so it only exercises
    // the plain cancel path, not this gate.)
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    const wrapper = screen.getByTestId('artist-node-tooltip')
    fireEvent.mouseEnter(wrapper) // overTooltipRef = true (pointer on tooltip)
    act(() => forceGraphProps.onNodeHover(null)) // canvas hover-out re-arms the dismiss
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).toBeInTheDocument() // gate bailed — tooltip survived under the cursor

    // A real pointer-leave then dismisses it.
    fireEvent.mouseLeave(wrapper)
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).not.toBeInTheDocument()
  })

  it('stays open when the cursor returns from the tooltip back onto its node', () => {
    // The "over EITHER node or tooltip" half of AC #2: leaving the tooltip arms a
    // dismiss, but landing back on the node must cancel it (handleNodeHover).
    renderViz()
    act(() => forceGraphProps.onNodeHover(satellite))
    const wrapper = screen.getByTestId('artist-node-tooltip')
    fireEvent.mouseEnter(wrapper)
    fireEvent.mouseLeave(wrapper) // arms the dismiss
    act(() => forceGraphProps.onNodeHover(satellite)) // back onto the node → cancelDismiss
    act(() => vi.advanceTimersByTime(300))
    expect(tooltipLink()).toBeInTheDocument()
  })
})

// PSY-1259: the node CLICK now expands (was re-center). Re-center moved to the tooltip
// (covered in ArtistNodeTooltip.test.tsx); here we lock the click→expand wiring.
describe('ArtistGraphVisualization — expand gesture (PSY-1259)', () => {
  beforeEach(() => vi.useRealTimers())
  afterEach(() => { forceGraphProps = null })

  it('fires onExpand (not onRecenter) when a satellite node is clicked', () => {
    const onExpand = vi.fn()
    const onRecenter = vi.fn()
    renderWithProviders(
      <ArtistGraphVisualization
        data={graphData}
        activeTypes={new Set(['similar'])}
        containerWidth={1024}
        onExpand={onExpand}
        onRecenter={onRecenter}
      />,
    )
    act(() => forceGraphProps.onNodeClick(satellite))
    expect(onExpand).toHaveBeenCalledWith({ id: 2, slug: 'frozen-soul', name: 'Frozen Soul' })
    expect(onRecenter).not.toHaveBeenCalled() // re-center is the tooltip action now, not the click
  })

  it('uses tool-surface selection mode for both satellite and center nodes', () => {
    const onSelect = vi.fn()
    const onExpand = vi.fn()
    renderWithProviders(
      <ArtistGraphVisualization
        data={graphData}
        activeTypes={new Set(['similar'])}
        containerWidth={1024}
        onSelect={onSelect}
        onExpand={onExpand}
      />,
    )

    act(() => forceGraphProps.onNodeClick(satellite))
    act(() => forceGraphProps.onNodeClick(centerNode))

    expect(onSelect).toHaveBeenNthCalledWith(1, satellite)
    expect(onSelect).toHaveBeenNthCalledWith(2, centerNode)
    expect(onExpand).not.toHaveBeenCalled()
  })

  it('notifies the caller after a background click', () => {
    const onBackgroundClick = vi.fn()
    renderWithProviders(
      <ArtistGraphVisualization
        data={graphData}
        activeTypes={new Set(['similar'])}
        containerWidth={1024}
        onBackgroundClick={onBackgroundClick}
      />,
    )
    act(() => forceGraphProps.onBackgroundClick())
    expect(onBackgroundClick).toHaveBeenCalledTimes(1)
  })

  it('clicking the center node is a no-op (the ego anchor is already expanded)', () => {
    const onExpand = vi.fn()
    renderWithProviders(
      <ArtistGraphVisualization
        data={graphData}
        activeTypes={new Set(['similar'])}
        containerWidth={1024}
        onExpand={onExpand}
      />,
    )
    act(() => forceGraphProps.onNodeClick(centerNode))
    expect(onExpand).not.toHaveBeenCalled()
  })
})
