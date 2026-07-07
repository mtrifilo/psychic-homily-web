import { describe, it, expect } from 'vitest'
import {
  compareScenesByActivity,
  DOT_COLOR_BASE,
  DOT_COLOR_HOVERED,
  DOT_COLOR_SELECTED,
  LABEL_TOP_K_FLOOR,
  LABEL_DECLUTTER_KM_BY_MIN_COUNT,
  labelDeclutterRadiusKm,
  DOT_COLOR_FOLLOWED,
  labelMinCountForAltitude,
  RING_ALTITUDE,
  sceneDotAltitude,
  sceneDotColor,
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
    // Cap = base 0.28 + variable cap 0.22 = 0.5 (PSY-1324 shrank it from 0.78 —
    // the old capped disc still covered ~130 km at Chicago and swallowed Milwaukee).
    expect(sceneDotRadius(283)).toBeCloseTo(0.5, 5)
    expect(sceneDotRadius(10_000)).toBeCloseTo(0.5, 5)
  })

  it('caps dense scenes far below the old uncapped sqrt formula (the PSY-1223 fix)', () => {
    // Old: 0.28 + sqrt(283)/14 ≈ 1.48 — a dot that swallowed its neighbours.
    const oldUncapped = 0.28 + Math.sqrt(283) / 14
    expect(oldUncapped).toBeGreaterThan(1.4)
    expect(sceneDotRadius(283)).toBeLessThan(oldUncapped)
    expect(sceneDotRadius(283)).toBeLessThanOrEqual(0.5)
  })

  it('keeps the dense-tier boundary at ≈49 shows despite the smaller cap (PSY-1324)', () => {
    // The divisor scaled with the cap (14 → 32) so the cap is still reached at
    // count ≈ (0.22×32)² ≈ 49 — the PSY-1223 "same max dot past ~49 shows"
    // calibration survives; only the cap's geometry shrank.
    expect(sceneDotRadius(40)).toBeLessThan(0.5)
    expect(sceneDotRadius(50)).toBeCloseTo(0.5, 5)
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

describe('sceneDotAltitude', () => {
  it('gives less-dense dots a taller cylinder so they stack above denser ones', () => {
    // The PSY-1324 occlusion fix: depth-tested cylinders mean equal heights let
    // the larger disc swallow a smaller neighbor; inverse-count height makes
    // the less-dense dot's top face always render above the denser one's.
    expect(sceneDotAltitude(0)).toBeGreaterThan(sceneDotAltitude(10))
    expect(sceneDotAltitude(10)).toBeGreaterThan(sceneDotAltitude(283))
  })

  it('still orders CAPPED dense scenes by count (the co-dense-neighbors case)', () => {
    // The offset is keyed to the raw count, not the capped radius: two adjacent
    // metros both past DOT_CAP_COUNT render equal-size dots, and a radius-keyed
    // offset would z-fight them. 50 and 283 are both capped yet must stack.
    expect(sceneDotRadius(50)).toBeCloseTo(sceneDotRadius(283), 5)
    expect(sceneDotAltitude(50)).toBeGreaterThan(sceneDotAltitude(283))
    expect(sceneDotAltitude(283)).toBeGreaterThan(sceneDotAltitude(10_000))
  })

  it('keeps the whole range subtle and above the pulse rings', () => {
    // Range = base 0.008 + (0, 0.008] — max at count 0.
    expect(sceneDotAltitude(0)).toBeCloseTo(0.016, 5)
    for (const c of [0, 5, 49, 283, 10_000, NaN]) {
      // Structural invariant: GlobeCanvas binds ringAltitude to RING_ALTITUDE.
      expect(sceneDotAltitude(c)).toBeGreaterThan(RING_ALTITUDE)
    }
  })

  it('treats non-finite counts like zero (inherits the radius guard)', () => {
    expect(sceneDotAltitude(NaN)).toBeCloseTo(sceneDotAltitude(0), 5)
    expect(sceneDotAltitude(undefined as unknown as number)).toBeCloseTo(sceneDotAltitude(0), 5)
  })
})

describe('sceneDotColor followed state (PSY-1340)', () => {
  const followed = new Set(['phoenix-az'])

  it('tints followed dots, with selected/hovered still winning', () => {
    expect(sceneDotColor('phoenix-az', null, null, followed)).toBe(DOT_COLOR_FOLLOWED)
    expect(sceneDotColor('phoenix-az', 'phoenix-az', null, followed)).toBe(DOT_COLOR_HOVERED)
    expect(sceneDotColor('phoenix-az', null, 'phoenix-az', followed)).toBe(DOT_COLOR_SELECTED)
    expect(sceneDotColor('denver-co', null, null, followed)).toBe(DOT_COLOR_BASE)
  })

  it('treats an absent set as unfollowed (logged-out)', () => {
    expect(sceneDotColor('phoenix-az', null, null, null)).toBe(DOT_COLOR_BASE)
    expect(sceneDotColor('phoenix-az', null, null)).toBe(DOT_COLOR_BASE)
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

  it('excludes a sub-threshold neighbor via the COUNT gate at the continental threshold', () => {
    // At the default continental zoom (altitude 1.8 → threshold 120), St. Paul (95)
    // does not clear the count gate, so only Minneapolis (187) labels. This is the
    // COUNT gate, NOT the PSY-1330 proximity declutter — that fires only when BOTH
    // cities clear the gate (see the "proximity declutter" describe block below).
    // St. Paul's label returns as you zoom in (the threshold drops). The PSY-1229
    // floor must NOT disturb this: at least one scene clears the threshold, so no
    // fallback.
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

  it('does NOT proximity-declutter the floor — the K guarantee wins over overlap', () => {
    // A seasonal dip where the K densest happen to include an adjacent pair (~37 km
    // apart, inside the 60 km continental radius). The floor must still return K
    // labels (never-fewer-than-K, PSY-1229) even though those two would overlap —
    // the PSY-1330 declutter is deliberately skipped on this safety-net path.
    const adjacentA = {
      city: 'AdjA',
      upcoming_show_count: 90,
      latitude: 40.0,
      longitude: -75.0,
    }
    const adjacentB = {
      city: 'AdjB',
      upcoming_show_count: 80,
      latitude: 40.3,
      longitude: -75.2,
    }
    const rest = [
      { city: 'C', upcoming_show_count: 70 },
      { city: 'D', upcoming_show_count: 60 },
      { city: 'E', upcoming_show_count: 50 },
      { city: 'F', upcoming_show_count: 40 },
    ]
    const labelled = visibleLabelScenes([adjacentA, adjacentB, ...rest], 120)
    // Both adjacent cities survive AND the floor still returns exactly K.
    expect(labelled).toHaveLength(LABEL_TOP_K_FLOOR)
    expect(labelled.map((s) => s.city)).toEqual(['AdjA', 'AdjB', 'C', 'D', 'E'])
  })
})

describe('visibleLabelScenes — proximity declutter (PSY-1330)', () => {
  // Real coords: Minneapolis / St. Paul are ~14.5 km apart; Chicago is ~570 km
  // from Minneapolis. Both twin-city counts clear the multi-region band (40).
  const minneapolis = {
    city: 'Minneapolis',
    upcoming_show_count: 187,
    latitude: 44.98,
    longitude: -93.27,
  }
  const stPaul = {
    city: 'St. Paul',
    upcoming_show_count: 95,
    latitude: 44.95,
    longitude: -93.09,
  }
  const chicago = {
    city: 'Chicago',
    upcoming_show_count: 283,
    latitude: 41.88,
    longitude: -87.63,
  }

  it('drops the less-dense of an adjacent pair at the multi-region band (the AC case)', () => {
    // altitude 1.0 → threshold 40: Minneapolis (187) AND St. Paul (95) both clear
    // it and are ~14.5 km apart, so their labels overlap. The denser Minneapolis
    // keeps its label; St. Paul is suppressed. Chicago is far, so it's unaffected.
    const labelled = visibleLabelScenes([minneapolis, stPaul, chicago], 40)
    expect(labelled.map((s) => s.city)).toEqual(['Minneapolis', 'Chicago'])
    expect(labelled).not.toContain(stPaul)
  })

  it('keeps the DENSER scene regardless of input order', () => {
    // St. Paul listed first, but Minneapolis (denser) must win the collision.
    const labelled = visibleLabelScenes([stPaul, minneapolis], 40)
    expect(labelled).toEqual([minneapolis])
  })

  it('also declutters at the tight metro band (~15 km reach)', () => {
    // altitude 0.6 → threshold 10, radius 15 km: the ~14.5 km pair still collides.
    const labelled = visibleLabelScenes([minneapolis, stPaul], 10)
    expect(labelled).toEqual([minneapolis])
  })

  it('leaves a far-apart pair alone (Chicago is ~570 km from Minneapolis)', () => {
    const labelled = visibleLabelScenes([minneapolis, chicago], 40)
    expect(labelled.map((s) => s.city)).toEqual(['Minneapolis', 'Chicago'])
  })

  it('treats a NaN/Infinity coordinate like a missing one (kept, never suppresses)', () => {
    // hasFiniteCoords excludes NaN/Infinity, not just null/undefined (adversarial
    // round-1 fix): a malformed-coord scene is never a proximity-collision
    // candidate. The densest here has a NaN latitude — it must be kept AND must not
    // block the real Minneapolis/St. Paul pair from decluttering normally.
    const nanCity = {
      city: 'Malformed',
      upcoming_show_count: 200,
      latitude: NaN,
      longitude: -93.1,
    }
    const labelled = visibleLabelScenes([nanCity, minneapolis, stPaul], 40)
    // NaN city kept (unmeasurable); the real pair still declutters (St. Paul dropped).
    expect(labelled.map((s) => s.city)).toEqual(['Malformed', 'Minneapolis'])
  })

  it('does NOT declutter scenes without coordinates (count-only fixtures unchanged)', () => {
    // The pre-PSY-1330 behavior: no coords → nothing to measure → both kept. This
    // pins that the count-only tests above are not silently altered by the pass.
    const mplsNoCoords = { city: 'Minneapolis', upcoming_show_count: 187 }
    const stpNoCoords = { city: 'St. Paul', upcoming_show_count: 95 }
    expect(visibleLabelScenes([mplsNoCoords, stpNoCoords], 40)).toEqual([
      mplsNoCoords,
      stpNoCoords,
    ])
  })

  it('does not declutter at a band with no calibrated distance', () => {
    // minCount 50 isn't a real band (labelDeclutterRadiusKm → 0) but is below both
    // counts, so both clear the gate and the adjacent coord-bearing pair is left
    // intact — the declutter is gated on a calibrated per-band distance existing.
    expect(visibleLabelScenes([minneapolis, stPaul], 50)).toEqual([
      minneapolis,
      stPaul,
    ])
  })

  it('declutters a co-dense adjacent pair at the continental band (future-proofing)', () => {
    // Two cities that both clear the continental 120 and sit ~40 km apart (within
    // the 60 km continental reach) — the global-expansion case the AC anticipates.
    const bigA = {
      city: 'BigA',
      upcoming_show_count: 150,
      latitude: 40.0,
      longitude: -75.0,
    }
    const bigB = {
      city: 'BigB',
      upcoming_show_count: 130,
      latitude: 40.3,
      longitude: -75.2,
    }
    const labelled = visibleLabelScenes([bigA, bigB], 120)
    expect(labelled).toEqual([bigA]) // denser kept, closer-and-less-dense dropped
  })

  it('resolves an exact-count tie between co-located scenes by input order', () => {
    // Two equal-count cities within the band-40 radius: compareScenesByActivity
    // ties (returns 0), so the stable sort preserves input order and the
    // FIRST-listed survives. Pins the documented tie behavior — a future secondary
    // sort key in the shared compareScenesByActivity would flip this and fail here.
    const first = {
      city: 'First',
      upcoming_show_count: 100,
      latitude: 44.98,
      longitude: -93.27,
    }
    const second = {
      city: 'Second',
      upcoming_show_count: 100,
      latitude: 44.95,
      longitude: -93.09,
    }
    expect(visibleLabelScenes([first, second], 40)).toEqual([first])
    expect(visibleLabelScenes([second, first], 40)).toEqual([second])
  })
})

describe('labelDeclutterRadiusKm', () => {
  it('returns the calibrated per-band reach, widening with altitude', () => {
    expect(labelDeclutterRadiusKm(120)).toBe(60) // continental
    expect(labelDeclutterRadiusKm(40)).toBe(30) // multi-region
    expect(labelDeclutterRadiusKm(10)).toBe(15) // metro cluster
    // The reach must not shrink as the camera zooms OUT (higher band).
    expect(labelDeclutterRadiusKm(120)).toBeGreaterThanOrEqual(
      labelDeclutterRadiusKm(40),
    )
    expect(labelDeclutterRadiusKm(40)).toBeGreaterThanOrEqual(
      labelDeclutterRadiusKm(10),
    )
  })

  it('returns 0 (no declutter) for a band with no calibrated distance', () => {
    expect(labelDeclutterRadiusKm(0)).toBe(0)
    expect(labelDeclutterRadiusKm(100)).toBe(0)
    expect(labelDeclutterRadiusKm(-5)).toBe(0)
  })

  it('exposes the band map as the single tuning knob', () => {
    expect(LABEL_DECLUTTER_KM_BY_MIN_COUNT[40]).toBe(30)
  })

  it('covers every gated band labelMinCountForAltitude can return (the two maps cannot drift)', () => {
    // Every non-zero threshold the altitude→count step function can produce must
    // have a declutter reach, or that band silently ships overlapping labels. Sweep
    // the altitude range finely so a newly-added band boundary is caught here.
    const gatedThresholds = new Set<number>()
    for (let i = 0; i <= 60; i++) {
      const min = labelMinCountForAltitude(i * 0.05)
      if (min > 0) gatedThresholds.add(min)
    }
    expect(gatedThresholds.size).toBeGreaterThan(0)
    for (const threshold of gatedThresholds) {
      expect(
        labelDeclutterRadiusKm(threshold),
        `gated band ${threshold} must have a declutter reach`,
      ).toBeGreaterThan(0)
    }
  })
})

describe('sceneDotColor (PSY-1312)', () => {
  it('base color when nothing is hovered or selected', () => {
    expect(sceneDotColor('phoenix-az', null, null)).toBe(DOT_COLOR_BASE)
  })

  it('hovered color for the hovered dot only', () => {
    expect(sceneDotColor('phoenix-az', 'phoenix-az', null)).toBe(DOT_COLOR_HOVERED)
    expect(sceneDotColor('mesa-az', 'phoenix-az', null)).toBe(DOT_COLOR_BASE)
  })

  it('selected color for the open-panel dot, persisting without hover', () => {
    expect(sceneDotColor('phoenix-az', null, 'phoenix-az')).toBe(DOT_COLOR_SELECTED)
  })

  it('selected wins over hovered on the same dot (no flicker re-hovering the open scene)', () => {
    expect(sceneDotColor('phoenix-az', 'phoenix-az', 'phoenix-az')).toBe(
      DOT_COLOR_SELECTED,
    )
  })

  it('hovered and selected can differ simultaneously', () => {
    expect(sceneDotColor('mesa-az', 'mesa-az', 'phoenix-az')).toBe(DOT_COLOR_HOVERED)
    expect(sceneDotColor('phoenix-az', 'mesa-az', 'phoenix-az')).toBe(
      DOT_COLOR_SELECTED,
    )
  })
})

describe('sceneDotColor genre tint base (PSY-1315)', () => {
  const GENRE = '#e0b66e' // a resolved family hex, standing in for genreFamilyColor(...)

  it('uses the genre base color when nothing is hovered/selected/followed', () => {
    expect(sceneDotColor('phoenix-az', null, null, null, GENRE)).toBe(GENRE)
  })

  it('falls back to the default orange when the scene has no genre base', () => {
    expect(sceneDotColor('phoenix-az', null, null, null, undefined)).toBe(DOT_COLOR_BASE)
  })

  it('hover/select/follow all override the genre base', () => {
    const followed = new Set(['phoenix-az'])
    expect(sceneDotColor('phoenix-az', 'phoenix-az', null, null, GENRE)).toBe(DOT_COLOR_HOVERED)
    expect(sceneDotColor('phoenix-az', null, 'phoenix-az', null, GENRE)).toBe(DOT_COLOR_SELECTED)
    expect(sceneDotColor('phoenix-az', null, null, followed, GENRE)).toBe(DOT_COLOR_FOLLOWED)
  })
})

describe('compareScenesByActivity', () => {
  it('orders liveliest first', () => {
    const scenes = [
      { upcoming_show_count: 3 },
      { upcoming_show_count: 283 },
      { upcoming_show_count: 41 },
    ]
    expect(
      scenes.sort(compareScenesByActivity).map((s) => s.upcoming_show_count),
    ).toEqual([283, 41, 3])
  })

  it('sorts non-finite counts last with a total order (no NaN comparator poisoning)', () => {
    const scenes = [
      { upcoming_show_count: NaN },
      { upcoming_show_count: 3 },
      { upcoming_show_count: Number.NaN },
      { upcoming_show_count: 41 },
    ]
    expect(
      scenes.sort(compareScenesByActivity).map((s) => s.upcoming_show_count),
    ).toEqual([41, 3, NaN, NaN])
    // Two malformed rows compare equal, not NaN (-Infinity − -Infinity trap).
    expect(
      compareScenesByActivity(
        { upcoming_show_count: NaN },
        { upcoming_show_count: NaN },
      ),
    ).toBe(0)
  })
})
