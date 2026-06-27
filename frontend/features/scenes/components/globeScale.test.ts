import { describe, it, expect } from 'vitest'
import {
  labelMinCountForAltitude,
  sceneDotRadius,
  sceneLabelSize,
  visibleLabelScenes,
} from './globeScale'

// PSY-1223: the globe's dot/label sizes were an UNCAPPED sqrt(count), so very
// dense scenes ballooned; and every scene carried an always-on label, so
// adjacent dense cities overlapped at the continental zoom. These tests pin the
// cap + the zoom-gating that fix both.

describe('sceneDotRadius', () => {
  it('returns the base radius for a scene with no upcoming shows', () => {
    expect(sceneDotRadius(0)).toBeCloseTo(0.28, 5)
  })

  it('grows with count but never exceeds the cap', () => {
    expect(sceneDotRadius(1)).toBeGreaterThan(sceneDotRadius(0))
    expect(sceneDotRadius(50)).toBeGreaterThan(sceneDotRadius(1))
    // Cap = base 0.28 + variable cap 0.5 = 0.78.
    expect(sceneDotRadius(283)).toBeCloseTo(0.78, 5)
    expect(sceneDotRadius(10_000)).toBeCloseTo(0.78, 5)
  })

  it('caps dense scenes far below the old uncapped sqrt formula (the PSY-1223 fix)', () => {
    // Old: 0.28 + sqrt(283)/14 ≈ 1.48 — a dot that swallowed its neighbours.
    const oldUncapped = 0.28 + Math.sqrt(283) / 14
    expect(oldUncapped).toBeGreaterThan(1.4)
    expect(sceneDotRadius(283)).toBeLessThan(oldUncapped)
    expect(sceneDotRadius(283)).toBeLessThanOrEqual(0.78)
  })

  it('is monotonic non-decreasing in count', () => {
    let prev = -Infinity
    for (const c of [0, 1, 5, 20, 49, 100, 283]) {
      const r = sceneDotRadius(c)
      expect(r).toBeGreaterThanOrEqual(prev)
      prev = r
    }
  })

  it('clamps a negative count to the base (defensive)', () => {
    expect(sceneDotRadius(-5)).toBeCloseTo(sceneDotRadius(0), 5)
  })
})

describe('sceneLabelSize', () => {
  it('returns the base size at zero and caps the high end', () => {
    expect(sceneLabelSize(0)).toBeCloseTo(0.5, 5)
    // Cap = base 0.5 + variable cap 0.35 = 0.85.
    expect(sceneLabelSize(283)).toBeCloseTo(0.85, 5)
    expect(sceneLabelSize(10_000)).toBeCloseTo(0.85, 5)
  })

  it('is monotonic non-decreasing and bounded by the cap', () => {
    let prev = -Infinity
    for (const c of [0, 10, 60, 125, 283]) {
      const s = sceneLabelSize(c)
      expect(s).toBeGreaterThanOrEqual(prev)
      expect(s).toBeLessThanOrEqual(0.85)
      prev = s
    }
  })
})

describe('labelMinCountForAltitude', () => {
  it('raises the label threshold as the camera zooms out', () => {
    expect(labelMinCountForAltitude(1.8)).toBe(120) // continental (default POV)
    expect(labelMinCountForAltitude(1.5)).toBe(120)
    expect(labelMinCountForAltitude(1.2)).toBe(40) // multi-region
    expect(labelMinCountForAltitude(0.8)).toBe(10) // metro cluster
    expect(labelMinCountForAltitude(0.4)).toBe(0) // zoomed in — all labels
  })

  it('is monotonic non-decreasing in altitude (zoom out never reveals MORE labels)', () => {
    let prev = -Infinity
    // Walk from zoomed-in (low altitude) to zoomed-out (high altitude).
    for (const alt of [0.2, 0.6, 0.9, 1.0, 1.4, 1.5, 1.8, 2.5]) {
      const min = labelMinCountForAltitude(alt)
      expect(min).toBeGreaterThanOrEqual(prev)
      prev = min
    }
  })
})

describe('visibleLabelScenes', () => {
  const minneapolis = { city: 'Minneapolis', upcoming_show_count: 187 }
  const stPaul = { city: 'St. Paul', upcoming_show_count: 95 }
  const chicago = { city: 'Chicago', upcoming_show_count: 283 }
  const duluth = { city: 'Duluth', upcoming_show_count: 6 }
  const scenes = [minneapolis, stPaul, chicago, duluth]

  it('returns every scene when the threshold is zero (fully zoomed in)', () => {
    expect(visibleLabelScenes(scenes, 0)).toEqual(scenes)
  })

  it('keeps only scenes at or above the threshold', () => {
    expect(visibleLabelScenes(scenes, 100)).toEqual([minneapolis, chicago])
    expect(visibleLabelScenes(scenes, 10)).toEqual([minneapolis, stPaul, chicago])
  })

  it('declutters an adjacent dense pair at the continental threshold (the AC case)', () => {
    // At the default continental zoom (altitude 1.8 → threshold 120), of the
    // ~10mi-apart Minneapolis/St. Paul pair only the denser Minneapolis keeps its
    // label, so the two are distinguishable without overlap. St. Paul's label
    // returns as you zoom in (threshold drops).
    const continental = labelMinCountForAltitude(1.8)
    const labelled = visibleLabelScenes([minneapolis, stPaul], continental)
    expect(labelled).toEqual([minneapolis])
    expect(labelled).not.toContain(stPaul)
  })
})
