import { describe, it, expect, vi } from 'vitest'
import type { GraphPalette } from './graphPalette'
import {
  drawIsolateShelfBand,
  drawIsolateShelfCaption,
  isolateShelfCaption,
  isolateShelfGeometry,
} from './isolateShelf'

// PSY-1454 — labeled isolate shelf (locked grammar decision 4): the shelf
// geometry is the single source shared with ForceGraphView's pinning effect,
// the band is foreground ink at ~3.5% with a 1-screen-px hairline, and the
// caption is the approved "+{N} not yet connected artists" copy drawn with
// the theme-aware halo recipe in muted ink.

const palette: GraphPalette = {
  edges: {},
  unknownEdge: '#888888',
  chart: [],
  otherCluster: '#94A3B8',
  labelText: '#eee7d9',
  labelHalo: '#0d0805',
  primary: '#e89960',
  mutedForeground: '#9c8c7c',
}

function makeCtx() {
  return {
    save: vi.fn(),
    restore: vi.fn(),
    // Deterministic width from the current font px (jsdom canvas returns 0):
    // ~0.6em per glyph, the same heuristic the graphLabels tests use.
    measureText(this: { font: string }, text: string) {
      const px = Number(/(\d+(?:\.\d+)?)px/.exec(this.font)?.[1] ?? 10)
      return { width: text.length * px * 0.6 }
    },
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    strokeText: vi.fn(),
    fillText: vi.fn(),
    font: '',
    textAlign: '',
    textBaseline: '',
    lineJoin: '',
    lineWidth: 0,
    strokeStyle: '',
    fillStyle: '',
  } as unknown as CanvasRenderingContext2D
}

describe('isolateShelfGeometry', () => {
  it('matches the pinning contract: y at 42% height, shelf at ±40% width', () => {
    expect(isolateShelfGeometry(1000, 500)).toEqual({
      y: 210,
      startX: -400,
      endX: 400,
    })
  })
})

describe('isolateShelfCaption', () => {
  it('uses the approved plural template', () => {
    expect(isolateShelfCaption(37)).toBe('+37 not yet connected artists')
  })

  it('reads grammatically for exactly one isolate', () => {
    expect(isolateShelfCaption(1)).toBe('+1 not yet connected artist')
  })
})

describe('drawIsolateShelfBand', () => {
  it('fills a band spanning past the shelf extents in foreground ink at ~3.5%', () => {
    const ctx = makeCtx()
    const geometry = isolateShelfGeometry(1000, 500)
    drawIsolateShelfBand(ctx, palette, geometry, 1)
    const fillRect = ctx.fillRect as ReturnType<typeof vi.fn>
    expect(fillRect).toHaveBeenCalledTimes(1)
    const [x, y, w, h] = fillRect.mock.calls[0]
    // Band contains the whole dot row (with padding) and straddles the dot y.
    expect(x).toBeLessThan(geometry.startX)
    expect(x + w).toBeGreaterThan(geometry.endX)
    expect(y).toBeLessThan(geometry.y)
    expect(y + h).toBeGreaterThan(geometry.y)
  })

  it('strokes a hairline along the band top at a constant 1 screen px', () => {
    const ctx = makeCtx()
    const geometry = isolateShelfGeometry(1000, 500)
    drawIsolateShelfBand(ctx, palette, geometry, 2)
    expect(ctx.stroke).toHaveBeenCalledTimes(1)
    expect(ctx.lineWidth).toBe(1 / 2)
    const moveTo = (ctx.moveTo as ReturnType<typeof vi.fn>).mock.calls[0]
    const lineTo = (ctx.lineTo as ReturnType<typeof vi.fn>).mock.calls[0]
    // Horizontal line along the band's top edge.
    expect(moveTo[1]).toBe(lineTo[1])
    expect(moveTo[1]).toBeLessThan(geometry.y)
  })

  it('derives both band and hairline color from the theme palette foreground', () => {
    const ctx = makeCtx()
    const fills: string[] = []
    const strokes: string[] = []
    Object.defineProperty(ctx, 'fillStyle', {
      set: v => fills.push(v as string),
    })
    Object.defineProperty(ctx, 'strokeStyle', {
      set: v => strokes.push(v as string),
    })
    drawIsolateShelfBand(ctx, palette, isolateShelfGeometry(1000, 500), 1)
    expect(fills).toEqual([`${palette.labelText}09`]) // ≈ 3.5% alpha
    expect(strokes).toEqual([`${palette.labelText}26`]) // hairline ≈ 15%
  })

  it('scopes its ctx mutations in save/restore', () => {
    const ctx = makeCtx()
    drawIsolateShelfBand(ctx, palette, isolateShelfGeometry(1000, 500), 1)
    expect(ctx.save).toHaveBeenCalledTimes(1)
    expect(ctx.restore).toHaveBeenCalledTimes(1)
  })
})

describe('drawIsolateShelfCaption', () => {
  it('draws the caption halo-then-fill, left-aligned at the shelf edge in muted ink', () => {
    const ctx = makeCtx()
    const geometry = isolateShelfGeometry(1000, 500)
    const fills: string[] = []
    const strokes: string[] = []
    Object.defineProperty(ctx, 'fillStyle', {
      set: v => fills.push(v as string),
    })
    Object.defineProperty(ctx, 'strokeStyle', {
      set: v => strokes.push(v as string),
    })
    drawIsolateShelfCaption(ctx, palette, geometry, 37, 1)
    expect(ctx.textAlign).toBe('left')
    expect(ctx.strokeText).toHaveBeenCalledWith(
      '+37 not yet connected artists',
      expect.any(Number),
      expect.any(Number)
    )
    const [text, x, y] = (ctx.fillText as ReturnType<typeof vi.fn>).mock
      .calls[0]
    expect(text).toBe('+37 not yet connected artists')
    // Anchored near the shelf's left edge, above the dot row.
    expect(x).toBeLessThan(geometry.startX)
    expect(y).toBeLessThan(geometry.y)
    expect(strokes).toEqual([palette.labelHalo])
    expect(fills).toEqual([palette.mutedForeground])
  })

  it('counter-scales the font to a constant screen size', () => {
    const ctx = makeCtx()
    drawIsolateShelfCaption(ctx, palette, isolateShelfGeometry(1000, 500), 5, 2)
    expect(ctx.font).toContain(`${12 / 2}px`)
  })

  // PSY-1456 (deferred LOW from the PSY-1454 adversarial review): on a very
  // narrow container at min zoom the counter-scaled caption grew wider than
  // the band in world units and spilled past its right edge. The draw now
  // measures the text and shrinks the font just enough to fit the band.
  it('shrinks the caption to fit the band on a narrow container at min zoom', () => {
    const ctx = makeCtx()
    const geometry = isolateShelfGeometry(400, 500)
    drawIsolateShelfCaption(ctx, palette, geometry, 5, 0.4)
    const fontPx = Number(/(\d+(?:\.\d+)?)px/.exec((ctx as unknown as { font: string }).font)?.[1])
    // Unclamped it would be the 26 world-px cap; the measured clamp shrinks it…
    expect(fontPx).toBeLessThan(26)
    // …so the measured text exactly fits the band's inner width (band spans
    // ±0.4*400 plus 32px padding each side, minus the mirrored 16px inset).
    const maxWidth = 400 * 0.8 + 2 * 32 - 2 * 16
    const text = '+5 not yet connected artists'
    expect(text.length * fontPx * 0.6).toBeCloseTo(maxWidth, 5)
  })

  it('leaves the caption font at the world-px cap when the band is wide enough', () => {
    const ctx = makeCtx()
    drawIsolateShelfCaption(ctx, palette, isolateShelfGeometry(1000, 500), 5, 0.4)
    expect((ctx as unknown as { font: string }).font).toContain('26px')
  })

  it('draws nothing for a zero or negative count', () => {
    const ctx = makeCtx()
    drawIsolateShelfCaption(ctx, palette, isolateShelfGeometry(1000, 500), 0, 1)
    expect(ctx.fillText).not.toHaveBeenCalled()
    expect(ctx.strokeText).not.toHaveBeenCalled()
  })
})
