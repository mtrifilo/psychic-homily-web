import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'

// PSY-1235 / PSY-1226: lock in the render-loop animation control, which is otherwise only
// verifiable in a real browser (jsdom can't run the rAF loop). Two behaviors:
//   - PSY-1235: ForceGraphView calls resumeAnimation() on (re-)mount so the loop revives
//     after a client back-navigation re-mount (which otherwise leaves it dead → frozen graph).
//   - PSY-1226: that resume is gated on !reducedMotion, so it does NOT defeat the
//     prefers-reduced-motion pauseAnimation() (the loop reschedules unconditionally, so an
//     ungated resume would keep it running forever).
// A regression that ungated the resume, or dropped it, would silently re-break one of the two
// tickets with nothing else failing.

// Capture the graph instance's imperative API so the test can assert the lifecycle calls.
// vi.hoisted so it's available inside the (hoisted) vi.mock factories below.
const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    // Called by the cluster-force effect; returns undefined so the strength guards skip.
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
  },
  reducedMotion: { value: false },
}))

// react-force-graph-2d loads via next/dynamic (ssr:false). Replace it with a forwardRef stub
// that exposes our spy API as the graph ref — so `graphRef.current.resumeAnimation()` etc. in
// the effects hit our spies (the real lib never mounts in jsdom).
vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(_props: Record<string, unknown>, ref: React.Ref<unknown>) {
        React.useImperativeHandle(ref, () => h.graph)
        return React.createElement('div', { 'data-testid': 'force-graph' })
      }),
  }
})

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => h.reducedMotion.value,
}))

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [{ id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 }]

const renderGraph = () =>
  renderWithProviders(
    <ForceGraphView nodes={nodes} links={[]} containerWidth={1024} ariaLabel="test graph" onNodeClick={() => {}} />,
  )

describe('ForceGraphView render-loop control (PSY-1235 / PSY-1226)', () => {
  beforeEach(() => {
    h.graph.pauseAnimation.mockClear()
    h.graph.resumeAnimation.mockClear()
    h.reducedMotion.value = false
  })

  it('resumes the render loop on mount for non-reduced-motion users', () => {
    renderGraph()
    expect(h.graph.resumeAnimation).toHaveBeenCalled()
    expect(h.graph.pauseAnimation).not.toHaveBeenCalled()
  })

  it('resumes again on a fresh re-mount — the PSY-1235 back-nav revive path', () => {
    // A browser back-navigation re-mounts the component; the effect must re-run and resume
    // so the loop (and all interaction) revives. jsdom can't run the rAF loop, but it can
    // verify the resume call fires on each fresh mount (an unmount+remount cycle), which is
    // what the effect's per-mount re-run guarantees. (The actual 0→159 fps revival is
    // browser-verified.)
    const { unmount } = renderGraph()
    expect(h.graph.resumeAnimation).toHaveBeenCalledTimes(1)
    h.graph.resumeAnimation.mockClear()
    unmount()
    renderGraph()
    expect(h.graph.resumeAnimation).toHaveBeenCalledTimes(1)
  })

  it('does NOT resume — and pauses — under prefers-reduced-motion (PSY-1226 gate holds)', () => {
    h.reducedMotion.value = true
    renderGraph()
    expect(h.graph.resumeAnimation).not.toHaveBeenCalled()
    expect(h.graph.pauseAnimation).toHaveBeenCalled()
  })
})
