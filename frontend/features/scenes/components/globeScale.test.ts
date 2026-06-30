import { describe, it, expect } from 'vitest'
import {
  LABEL_TOP_K_FLOOR,
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

  it('treats negative or non-finite counts as the base (defensive)', () => {
    expect(sceneDotRadius(-5)).toBeCloseTo(0.28, 5)
    // NaN/undefined (the type says number, but the API field could be missing) must
    // NOT propagate a NaN radius into the merged three.js point geometry.
    expect(sceneDotRadius(NaN)).toBeCloseTo(0.28, 5)
    expect(sceneDotRadius(undefined as unknown as number)).toBeCloseTo(0.28, 5)
  })
})

describe('sceneLabelSize', () => {
  it('returns the base size at zero, caps the high end, and guards non-finite', () => {
    expect(sceneLabelSize(0)).toBeCloseTo(0.5, 5)
    // Cap = base 0.5 + variable cap 0.35 = 0.85.
    expect(sceneLabelSize(283)).toBeCloseTo(0.85, 5)
    expect(sceneLabelSize(10_000)).toBeCloseTo(0.85, 5)
    expect(sceneLabelSize(NaN)).toBeCloseTo(0.5, 5)
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
    expect(labelMinCountForAltitude(1.6)).toBe(120) // geo-resolved POV (AtlasGlobe)
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
    // returns as you zoom in (threshold drops). The PSY-1229 floor must NOT
    // disturb this: at least one scene clears the threshold, so no fallback.
    const continental = labelMinCountForAltitude(1.8)
    const labelled = visibleLabelScenes([minneapolis, stPaul], continental)
    expect(labelled).toEqual([minneapolis])
    expect(labelled).not.toContain(stPaul)
  })
})

describe('visibleLabelScenes — top-K quiet-season floor (PSY-1229)', () => {
  // A quiet stretch where seasonal counts have all dipped below the continental
  // threshold (120). Before the floor this rendered ZERO labels (the bug).
  const quiet = [
    { city: 'A', upcoming_show_count: 90 },
    { city: 'B', upcoming_show_count: 80 },
    { city: 'C', upcoming_show_count: 70 },
    { city: 'D', upcoming_show_count: 60 },
    { city: 'E', upcoming_show_count: 50 },
    { city: 'F', upcoming_show_count: 40 },
    { city: 'G', upcoming_show_count: 30 },
  ]

  it('falls back to the top-K densest when nothing clears the threshold (never empty)', () => {
    const labelled = visibleLabelScenes(quiet, 120)
    expect(labelled).toHaveLength(LABEL_TOP_K_FLOOR)
    expect(labelled.map((s) => s.city)).toEqual(['A', 'B', 'C', 'D', 'E'])
  })

  it('returns all scenes when fewer than K exist and none clear the threshold', () => {
    const few = quiet.slice(0, 3)
    expect(visibleLabelScenes(few, 120)).toEqual(few)
  })

  it('returns exactly K at the slice boundary (K scenes, none clearing)', () => {
    const exactlyK = quiet.slice(0, LABEL_TOP_K_FLOOR)
    expect(visibleLabelScenes(exactlyK, 120)).toHaveLength(LABEL_TOP_K_FLOOR)
  })

  it('does NOT trigger when at least one scene clears the threshold (no re-clutter)', () => {
    // The calibrated sparse continental view must survive: one city clears 120,
    // so the floor stays out and no sub-threshold cities are pulled back in.
    const big = { city: 'Big', upcoming_show_count: 200 }
    expect(visibleLabelScenes([big, ...quiet], 120)).toEqual([big])
  })

  it('excludes non-finite counts from the floor (never floored in over a real scene)', () => {
    const withNaN = [
      { city: 'NaNville', upcoming_show_count: NaN },
      { city: 'Real1', upcoming_show_count: 30 },
      { city: 'Real2', upcoming_show_count: 20 },
    ]
    expect(visibleLabelScenes(withNaN, 120).map((s) => s.city)).toEqual([
      'Real1',
      'Real2',
    ])
  })

  it('returns empty when every count is NaN (nothing clears the gate, nothing real to floor in)', () => {
    const allNaN = [
      { city: 'X', upcoming_show_count: NaN },
      { city: 'Y', upcoming_show_count: NaN },
    ]
    expect(visibleLabelScenes(allNaN, 120)).toEqual([])
  })

  it('returns an empty array when there are no scenes at all', () => {
    expect(visibleLabelScenes([], 120)).toEqual([])
  })
})
