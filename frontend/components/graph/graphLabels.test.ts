import { describe, expect, it } from 'vitest'

import {
  LABEL_MIN_SCALE,
  TRUNCATE_KEEP_LENGTH,
  TRUNCATE_MAX_LENGTH,
  degreeMap,
  labelFontSize,
  renderGraphLabels,
  truncateLabel,
  type GraphLabelSpec,
} from './graphLabels'
import type { GraphPalette } from './graphPalette'

const PALETTE = { labelText: '#fff', labelHalo: '#000' } as unknown as GraphPalette

// Minimal fake 2d context: records the text passed to stroke/fill (and order),
// and computes a deterministic measureText width from the current `font` px size
// (jsdom canvas measureText returns 0, which would defeat collision tests).
function makeCtx() {
  const fills: string[] = []
  const order: string[] = []
  const ctx = {
    font: '10px sans-serif',
    textAlign: '',
    textBaseline: '',
    lineWidth: 0,
    lineJoin: '',
    strokeStyle: '',
    fillStyle: '',
    save() {},
    restore() {},
    measureText(t: string) {
      const px = Number(/(\d+(?:\.\d+)?)px/.exec(this.font)?.[1] ?? 10)
      return { width: t.length * px * 0.6 }
    },
    strokeText(t: string) {
      order.push(`stroke:${t}`)
    },
    fillText(t: string) {
      fills.push(t)
      order.push(`fill:${t}`)
    },
  }
  return { ctx: ctx as unknown as CanvasRenderingContext2D, fills, order }
}

function spec(p: Partial<GraphLabelSpec> & Pick<GraphLabelSpec, 'x' | 'y' | 'text'>): GraphLabelSpec {
  return { fontSize: 12, ...p }
}

describe('renderGraphLabels', () => {
  it('draws every label when none overlap', () => {
    const { ctx, fills } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 0, text: 'A' }),
      spec({ x: 1000, y: 0, text: 'B' }),
      spec({ x: 0, y: 500, text: 'C' }),
    ])
    expect(fills.sort()).toEqual(['A', 'B', 'C'])
  })

  it('skips a lower-priority label that overlaps a higher-priority one', () => {
    const { ctx, fills } = makeCtx()
    // Same position → boxes overlap; keep only the higher priority.
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 0, text: 'low', priority: 1 }),
      spec({ x: 0, y: 0, text: 'high', priority: 9 }),
    ])
    expect(fills).toEqual(['high'])
  })

  it('always draws a forced label and lets it cull an overlapping non-forced one', () => {
    const { ctx, fills } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [
      // High priority but NOT forced — should yield to the forced center.
      spec({ x: 0, y: 0, text: 'neighbor', priority: 99 }),
      spec({ x: 0, y: 0, text: 'center', force: true, priority: 0 }),
    ])
    expect(fills).toEqual(['center'])
  })

  it('draws two forced labels even when they overlap', () => {
    const { ctx, fills } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 0, text: 'one', force: true }),
      spec({ x: 0, y: 0, text: 'two', force: true }),
    ])
    expect(fills.sort()).toEqual(['one', 'two'])
  })

  it('keeps the higher-degree (priority) label when two overlap', () => {
    const { ctx, fills } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 0, text: 'leaf', priority: 1 }),
      spec({ x: 0, y: 0, text: 'hub', priority: 7 }),
    ])
    expect(fills).toEqual(['hub'])
  })

  it('culls a vertically-stacked label via the glyph-height factor', () => {
    // 'leaf' top is at y=16 — clear of 'hub's bare fontSize (12) but INSIDE its
    // 1.25x glyph-height box, so the height factor is what makes them collide.
    // If LABEL_HEIGHT_FACTOR dropped to 1.0 both would draw and this would fail.
    const { ctx, fills } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 16, text: 'leaf', fontSize: 12, priority: 1 }),
      spec({ x: 0, y: 0, text: 'hub', fontSize: 12, priority: 7 }),
    ])
    expect(fills).toEqual(['hub'])
  })

  it('strokes the halo before filling the text', () => {
    const { ctx, order } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [spec({ x: 0, y: 0, text: 'X' })])
    expect(order).toEqual(['stroke:X', 'fill:X'])
  })

  it('uses the requested numeric font weight for curated label tiers', () => {
    const { ctx } = makeCtx()
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 0, text: 'Headline', fontSize: 17, fontWeight: 600 }),
    ])
    expect(ctx.font).toBe('600 17px sans-serif')
  })

  it('skips empty/whitespace-only labels without reserving their collision box', () => {
    const { ctx, fills } = makeCtx()
    renderGraphLabels(ctx, PALETTE, []) // no-op on empty input
    // A blank/whitespace label must neither draw NOR cull a real neighbor at the
    // same position — otherwise it reserves an invisible box and hides the name.
    renderGraphLabels(ctx, PALETTE, [
      spec({ x: 0, y: 0, text: '   ', priority: 9 }),
      spec({ x: 0, y: 0, text: 'real', priority: 1 }),
    ])
    expect(fills).toEqual(['real'])
  })
})

// PSY-1445: the shared label constants exist so the two canvas primitives
// (ForceGraphView + ArtistGraphVisualization) can't drift apart again. These
// tests pin the agreed values — changing any of them is a deliberate,
// both-surfaces typography decision, not a local tweak.
describe('shared label constants (PSY-1445)', () => {
  it('pins the zoom gate at 0.7', () => {
    expect(LABEL_MIN_SCALE).toBe(0.7)
  })

  it('labelFontSize targets ~11 screen px, clamped to 9..13 graph px', () => {
    expect(labelFontSize(1)).toBe(11) // midrange: 11/scale
    expect(labelFontSize(0.5)).toBe(13) // far out: clamped at 13
    expect(labelFontSize(LABEL_MIN_SCALE)).toBe(13) // at the gate: clamp active
    expect(labelFontSize(2)).toBe(9) // far in: clamped at 9
    expect(labelFontSize(1.1)).toBeCloseTo(10) // inside the clamp: 11/scale
  })

  it('pins the truncation thresholds at 22/20', () => {
    expect(TRUNCATE_MAX_LENGTH).toBe(22)
    expect(TRUNCATE_KEEP_LENGTH).toBe(20)
  })

  it('truncateLabel keeps names up to 22 chars and cuts longer ones to 20 + ellipsis', () => {
    expect(truncateLabel('Short Name')).toBe('Short Name')
    const exactly22 = 'A'.repeat(TRUNCATE_MAX_LENGTH)
    expect(truncateLabel(exactly22)).toBe(exactly22)
    expect(truncateLabel('B'.repeat(23))).toBe('B'.repeat(TRUNCATE_KEEP_LENGTH) + '…')
    expect(truncateLabel('They Are Gutting a Body of Water')).toBe('They Are Gutting a B…')
  })
})

describe('degreeMap', () => {
  it('counts links per node id, handling both bare-id and resolved-node link ends', () => {
    // d3-force mutates link.source/target from a bare id to the resolved node
    // object in place; degreeMap must count the same id either way.
    const degrees = degreeMap<number>([
      { source: 1, target: 2 },
      { source: { id: 1 }, target: { id: 3 } },
    ])
    expect(degrees.get(1)).toBe(2)
    expect(degrees.get(2)).toBe(1)
    expect(degrees.get(3)).toBe(1)
    expect(degrees.get(99)).toBeUndefined()
  })
})
