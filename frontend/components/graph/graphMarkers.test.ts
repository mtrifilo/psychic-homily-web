import { describe, it, expect } from 'vitest'
import {
  PLAYABLE_RING_COLOR,
  PLAYABLE_RING_GAP,
  PLAYABLE_RING_WIDTH,
  UPCOMING_SHOW_DOT_COLOR,
  UPCOMING_SHOW_DOT_INSET,
  UPCOMING_SHOW_DOT_RADIUS,
  drawPlayableRing,
  drawUpcomingShowDot,
} from './graphMarkers'

// PSY-1453: the shared marker set exists so the ego graph and ForceGraphView
// can never drift on marker color/geometry again (the audit found show-dot
// radius 3 @ offset 2 vs 2.5 @ 1.5). These constants are the contract — a
// change here is a deliberate design change on EVERY graph surface.

interface RecordedArc {
  x: number
  y: number
  r: number
  fillStyle?: string
  strokeStyle?: string
  lineWidth?: number
}

function makeRecordingCtx() {
  const arcs: RecordedArc[] = []
  let current: RecordedArc | null = null
  const ctx = {
    fillStyle: '' as string,
    strokeStyle: '' as string,
    lineWidth: 0,
    beginPath() {
      current = null
    },
    arc(x: number, y: number, r: number) {
      current = { x, y, r }
    },
    fill() {
      if (current) arcs.push({ ...current, fillStyle: String(ctx.fillStyle) })
    },
    stroke() {
      if (current)
        arcs.push({ ...current, strokeStyle: String(ctx.strokeStyle), lineWidth: ctx.lineWidth })
    },
    arcs,
  }
  return ctx
}

describe('graphMarkers — pinned marker contract (PSY-1453)', () => {
  it('pins the marker colors', () => {
    expect(UPCOMING_SHOW_DOT_COLOR).toBe('#22c55e')
    expect(PLAYABLE_RING_COLOR).toBe('#a855f7')
  })

  it('pins the shared geometry (the audited drift: 2.5 @ 1.5 wins)', () => {
    expect(UPCOMING_SHOW_DOT_RADIUS).toBe(2.5)
    expect(UPCOMING_SHOW_DOT_INSET).toBe(1.5)
    expect(PLAYABLE_RING_GAP).toBe(2.5)
    expect(PLAYABLE_RING_WIDTH).toBe(1.5)
  })

  it('drawUpcomingShowDot fills a green dot at the node top-right corner', () => {
    const ctx = makeRecordingCtx()
    drawUpcomingShowDot(ctx as unknown as CanvasRenderingContext2D, 10, 20, 8)
    expect(ctx.arcs).toEqual([
      {
        x: 10 + 8 - UPCOMING_SHOW_DOT_INSET,
        y: 20 - 8 + UPCOMING_SHOW_DOT_INSET,
        r: UPCOMING_SHOW_DOT_RADIUS,
        fillStyle: UPCOMING_SHOW_DOT_COLOR,
      },
    ])
  })

  it('drawPlayableRing strokes a violet ring hugging the node', () => {
    const ctx = makeRecordingCtx()
    drawPlayableRing(ctx as unknown as CanvasRenderingContext2D, -4, 6, 12)
    expect(ctx.arcs).toEqual([
      {
        x: -4,
        y: 6,
        r: 12 + PLAYABLE_RING_GAP,
        strokeStyle: PLAYABLE_RING_COLOR,
        lineWidth: PLAYABLE_RING_WIDTH,
      },
    ])
  })
})
