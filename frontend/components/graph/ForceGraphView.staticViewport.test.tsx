import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'

// PSY-1344: staticViewport must translate into react-force-graph's
// interaction flags — zoom and pan disabled — so an embed surface (homepage
// graph section) never captures page scroll. Node drag is retired on EVERY
// surface (PSY-1452 locked grammar decision), so enableNodeDrag must be
// false regardless of mode. jsdom can't exercise real wheel/drag handling,
// so we lock in the prop contract at the ForceGraph2D boundary instead.

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

const nodes: GraphNode[] = [
  { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 },
  { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0 },
]

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
  it('defaults to zoom/pan enabled with node drag retired (PSY-1452)', () => {
    renderGraph()
    expect(h.lastProps.enableZoomInteraction).toBe(true)
    expect(h.lastProps.enablePanInteraction).toBe(true)
    expect(h.lastProps.enableNodeDrag).toBe(false)
  })

  it('disables zoom, pan, and node drag in static-viewport mode', () => {
    renderGraph(true)
    expect(h.lastProps.enableZoomInteraction).toBe(false)
    expect(h.lastProps.enablePanInteraction).toBe(false)
    expect(h.lastProps.enableNodeDrag).toBe(false)
  })

  // PSY-1452: drag is retired outright, so no drag handlers are wired —
  // the PSY-1447 drag-time cooldown re-arm and the PSY-1217 drag-dismiss
  // for tooltips went with them.
  it('wires no drag handlers on any surface (PSY-1452)', () => {
    renderGraph()
    expect(h.lastProps.onNodeDrag).toBeUndefined()
    expect(h.lastProps.onNodeDragEnd).toBeUndefined()
  })

  // PSY-1442 shipped the synchronous pre-settle (warmup on, visible settle
  // off) for static viewports; PSY-1447 generalized it to EVERY surface so
  // the first painted frame is final everywhere. With node drag retired
  // (PSY-1452), cooldownTicks is unconditionally 0 — the drag-time re-arm
  // was the only exception.
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
})
