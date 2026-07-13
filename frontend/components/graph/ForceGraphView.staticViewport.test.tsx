import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1344: staticViewport must translate into react-force-graph's
// interaction flags — zoom, pan, and node drag all disabled — so an embed
// surface (homepage graph section) never captures page scroll. jsdom can't
// exercise real wheel/drag handling, so we lock in the prop contract at the
// ForceGraph2D boundary instead.

const h = vi.hoisted(() => ({
  lastProps: {} as Record<string, unknown>,
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(props: Record<string, unknown>, _ref: React.Ref<unknown>) {
        h.lastProps = props
        return React.createElement('div', { 'data-testid': 'force-graph' })
      }),
  }
})

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [{ id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 }]

const renderGraph = (staticViewport?: boolean) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={[]}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
      staticViewport={staticViewport}
    />,
  )

beforeEach(() => {
  h.lastProps = {}
})

describe('ForceGraphView staticViewport (PSY-1344)', () => {
  it('defaults to full interaction (zoom, pan, drag enabled)', () => {
    renderGraph()
    expect(h.lastProps.enableZoomInteraction).toBe(true)
    expect(h.lastProps.enablePanInteraction).toBe(true)
    expect(h.lastProps.enableNodeDrag).toBe(true)
  })

  it('disables zoom, pan, and node drag in static-viewport mode', () => {
    renderGraph(true)
    expect(h.lastProps.enableZoomInteraction).toBe(false)
    expect(h.lastProps.enablePanInteraction).toBe(false)
    expect(h.lastProps.enableNodeDrag).toBe(false)
  })

  // PSY-1442 shipped the synchronous pre-settle (warmup on, visible settle
  // off) for static viewports; PSY-1447 generalized it to EVERY surface so
  // the first painted frame is final everywhere.
  it('pre-settles via warmupTicks with no cooldown in static-viewport mode (PSY-1442)', () => {
    renderGraph(true)
    expect(h.lastProps.warmupTicks).toBe(200)
    expect(h.lastProps.cooldownTicks).toBe(0)
  })

  it('pre-settles interactive surfaces the same way — the settle animation is retired (PSY-1447)', () => {
    renderGraph()
    expect(h.lastProps.warmupTicks).toBe(200)
    expect(h.lastProps.cooldownTicks).toBe(0)
  })

  // PSY-1447: cooldownTicks=0 at rest means the engine never ticks after the
  // synchronous warmup — verified against the real react-force-graph engine
  // (node_modules/force-graph): with cooldownTicks=0, a node-drag's
  // resetCountdown() is immediately followed by cntTicks(1) > cooldownTicks(0),
  // so forceLayout.tick() never runs and every neighbor freezes mid-drag,
  // leaving edges permanently stretched between the dragged node's live
  // position and its frozen neighbors after release (reproduced in a real
  // browser during manual repro — a materially worse regression than "less
  // lively" once actually seen). cooldownTicks re-arms to the pre-PSY-1442
  // interactive default for exactly the drag gesture's duration so dragging
  // keeps its pre-existing live neighbor-reflow; every other path (mount,
  // data digest, resize) stays governed by warmupTicks/zero-cooldown only.
  it('re-arms live ticking for the duration of a node drag, then re-freezes on drag end (PSY-1447)', () => {
    renderGraph()
    expect(h.lastProps.cooldownTicks).toBe(0)

    act(() => {
      ;(h.lastProps.onNodeDrag as () => void)()
    })
    expect(h.lastProps.cooldownTicks).toBe(200)

    act(() => {
      ;(h.lastProps.onNodeDragEnd as () => void)()
    })
    expect(h.lastProps.cooldownTicks).toBe(0)
  })
})
