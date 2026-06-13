import { describe, it, expect } from 'vitest'
import { alphaToHex } from './ForceGraphView'

// PSY-365 — covers the small public helpers callers reuse from the shared
// ForceGraphView. The canvas + d3-force pipeline are exercised end-to-end
// by SceneGraph.test.tsx + VenueBillNetwork.test.tsx (jsdom can't render
// canvas, so behaviour-level coverage lives there). The cluster palette
// moved to ./graphPalette.ts in PSY-1083 — see graphPalette.test.ts.

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
