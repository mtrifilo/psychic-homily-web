import { describe, it, expect } from 'vitest'
import { clusterColor, alphaToHex } from './ForceGraphView'

// PSY-365 — covers the small public helpers that callers (SceneGraph's
// cluster-pill legend, BillNetwork's future legend extension) reuse from
// the shared ForceGraphView. The canvas + d3-force pipeline are exercised
// end-to-end by SceneGraph.test.tsx + VenueBillNetwork.test.tsx (jsdom
// can't render canvas, so behaviour-level coverage lives there).

describe('clusterColor', () => {
  it('returns the Okabe-Ito hex at indices 0..7', () => {
    // We don't pin the exact hex values here — the palette is implementation
    // detail. We just verify each valid index returns a recognizable hex
    // string and they're all distinct (the colorblind audit lives in
    // graph-colorblind-audit.md).
    const colors = new Set<string>()
    for (let i = 0; i < 8; i++) {
      const c = clusterColor(i)
      expect(c).toMatch(/^#[0-9A-Fa-f]{6}$/)
      colors.add(c)
    }
    expect(colors.size).toBe(8)
  })

  it('returns the neutral grey for "other" / out-of-range indices', () => {
    const grey = clusterColor(-1)
    expect(grey).toMatch(/^#[0-9A-Fa-f]{6}$/)
    // Out-of-range falls back to the same neutral grey.
    expect(clusterColor(99)).toBe(grey)
    expect(clusterColor(8)).toBe(grey)
  })
})

describe('alphaToHex', () => {
  it('produces a 2-char lowercase hex pair for 0..1', () => {
    expect(alphaToHex(0)).toBe('00')
    expect(alphaToHex(1)).toBe('ff')
    // 0.5 → 128 → "80"
    expect(alphaToHex(0.5)).toBe('80')
    // 0.12 → 31 → "1f"
    expect(alphaToHex(0.12)).toBe('1f')
  })

  it('clamps out-of-range alpha values', () => {
    expect(alphaToHex(-1)).toBe('00')
    expect(alphaToHex(2)).toBe('ff')
  })
})
