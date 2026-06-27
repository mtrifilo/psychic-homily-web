import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// PSY-1226: ArtistGraph gates its palette/hover resumeAnimation() on !reducedMotion, so the
// resume can't defeat the prefers-reduced-motion pauseAnimation() (force-graph's rAF loop
// reschedules unconditionally — an ungated resume on mount would keep it running forever).
// jsdom can't run the loop, so we observe the lifecycle calls via a ref-forwarding stub.
// Mirrors ForceGraphView.reducedMotion.test.tsx; this guards the ArtistGraph copy of the gate.

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    zoomToFit: vi.fn(), // re-frame effect (fires via setTimeout; not exercised here)
  },
  reducedMotion: { value: false },
}))

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

import { ArtistGraphVisualization } from './ArtistGraph'

const data: ArtistGraph = {
  center: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', city: 'Phoenix', state: 'AZ', upcoming_show_count: 3 },
  nodes: [{ id: 2, name: 'Frozen Soul', slug: 'frozen-soul', city: 'Fort Worth', state: 'TX', upcoming_show_count: 1 }],
  links: [{ source_id: 1, target_id: 2, type: 'similar', score: 0.85, votes_up: 8, votes_down: 2 }],
  user_votes: {},
}

const renderGraph = () =>
  renderWithProviders(<ArtistGraphVisualization data={data} activeTypes={new Set(['similar'])} containerWidth={1024} />)

describe('ArtistGraph render-loop control (PSY-1226)', () => {
  beforeEach(() => {
    h.graph.pauseAnimation.mockClear()
    h.graph.resumeAnimation.mockClear()
    h.reducedMotion.value = false
  })

  it('resumes the render loop on mount for non-reduced-motion users (palette/hover repaint)', () => {
    renderGraph()
    expect(h.graph.resumeAnimation).toHaveBeenCalled()
    expect(h.graph.pauseAnimation).not.toHaveBeenCalled()
  })

  it('does NOT resume — and pauses — under prefers-reduced-motion (gate holds)', () => {
    h.reducedMotion.value = true
    renderGraph()
    expect(h.graph.resumeAnimation).not.toHaveBeenCalled()
    expect(h.graph.pauseAnimation).toHaveBeenCalled()
  })
})
