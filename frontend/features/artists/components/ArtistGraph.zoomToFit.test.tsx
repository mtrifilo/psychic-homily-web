import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// The ego canvas re-frames itself on a 250ms timer (the backstop for the
// next/dynamic canvas chunk, whose ref is undefined on the first mount pass).
// Two things about that call are contracts rather than incidental values:
//
//   - PADDING is keyed on the canvas HEIGHT, not the container width. Padding
//     competes with height, so the old width-keyed form gave the 70px desktop
//     padding to the 360px inline Connections section — ~40% of the box —
//     over-shrinking the ring toward the LABEL_MIN_SCALE cull.
//   - DURATION is instant for Section-class hosts (PSY-1447 pre-settle) and
//     tweened for the dialog, which opens over a backdrop.
//
// jsdom can't run the canvas, so the calls are observed through a
// ref-forwarding stub (same harness shape as ArtistGraph.reducedMotion).

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    zoomToFit: vi.fn(),
  },
  /**
   * When true the stub reproduces next/dynamic(ssr:false)'s TWO-PHASE mount:
   * the first client render paints the loading fallback and exposes NO
   * imperative handle, and the canvas attaches on a later render. That is the
   * real shape on a cold page load, and the shape the re-frame effect used to
   * bail on permanently.
   */
  deferMount: { value: false },
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')

  // The loaded chunk. Only this component ever touches the ref — mirroring
  // next/dynamic, where the loading fallback is a DIFFERENT element that
  // forwards nothing.
  const LoadedGraph = React.forwardRef(function LoadedGraph(
    _props: Record<string, unknown>,
    ref: React.Ref<unknown>
  ) {
    React.useImperativeHandle(ref, () => h.graph, [])
    return React.createElement('div', { 'data-testid': 'force-graph' })
  })

  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(
        props: Record<string, unknown>,
        ref: React.Ref<unknown>
      ) {
        const [chunkLoaded, setChunkLoaded] = React.useState(
          !h.deferMount.value
        )
        // Land the chunk in a LATER task, not in this commit's effect flush —
        // otherwise React re-renders inside the same flush and the ref is
        // already attached by the time the host's own mount effect runs, which
        // would hide the very bail-out being guarded against.
        React.useEffect(() => {
          if (chunkLoaded) return
          const t = setTimeout(() => setChunkLoaded(true), 0)
          return () => clearTimeout(t)
        }, [chunkLoaded])
        if (!chunkLoaded) {
          return React.createElement('div', { 'data-testid': 'graph-skeleton' })
        }
        return React.createElement(LoadedGraph, { ...props, ref })
      }),
  }
})

import { ArtistGraphVisualization } from './ArtistGraph'

const data: ArtistGraph = {
  center: {
    id: 1,
    name: 'Gatecreeper',
    slug: 'gatecreeper',
    upcoming_show_count: 0,
    has_playable_audio: false,
  },
  nodes: [
    {
      id: 2,
      name: 'Frozen Soul',
      slug: 'frozen-soul',
      upcoming_show_count: 0,
      has_playable_audio: false,
    },
  ],
  links: [
    {
      source_id: 1,
      target_id: 2,
      type: 'similar',
      score: 0.85,
      votes_up: 0,
      votes_down: 0,
    },
  ],
}

function renderGraph(
  props: Partial<React.ComponentProps<typeof ArtistGraphVisualization>> = {}
) {
  return renderWithProviders(
    <ArtistGraphVisualization
      data={data}
      activeTypes={new Set(['similar'])}
      containerWidth={1024}
      {...props}
    />
  )
}

describe('ArtistGraph zoomToFit framing (PSY-1548)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    h.graph.zoomToFit.mockClear()
    h.deferMount.value = false
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('gives the dialog its tweened re-frame and the 70px desktop padding', () => {
    renderGraph()
    vi.advanceTimersByTime(250)
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(500, 70)
  })

  it('keeps the 40px padding on the short mobile dialog canvas (350px)', () => {
    renderGraph({ containerWidth: 500 })
    vi.advanceTimersByTime(250)
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(500, 40)
  })

  it('drops to 40px padding for a short Section canvas even at desktop width', () => {
    // The inline Connections section: 360px tall in a ~768px main column —
    // the case the old width-keyed padding got wrong.
    renderGraph({ height: 360, containerWidth: 768 })
    vi.advanceTimersByTime(250)
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(500, 40)
  })

  it('frames instantly when the host opts into the Section pre-settle', () => {
    renderGraph({ height: 360, containerWidth: 768, instantFit: true })
    vi.advanceTimersByTime(250)
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(0, 40)
  })

  it('still frames when the lazy canvas attaches AFTER the first render', () => {
    // The regression: the re-frame effect keyed only on the data, so on a cold
    // mount it ran once against a null ref and never again — the graph was
    // never framed at all, and the ego ring overflowed the Section box at
    // whatever scale the engine happened to use.
    h.deferMount.value = true
    renderGraph({ height: 360, containerWidth: 768, instantFit: true })
    // Chunk lands (task boundary), THEN the host's 250ms settle delay elapses.
    act(() => {
      vi.advanceTimersByTime(0)
    })
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(0, 40)
  })
})
