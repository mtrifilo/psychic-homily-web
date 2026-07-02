import { describe, it, expect } from 'vitest'
import { pickDriftScene } from './drift'
import type { PlaceableScene } from './globeTypes'

function scene(slug: string, upcoming: number): PlaceableScene {
  return {
    city: slug,
    state: 'AZ',
    slug,
    venue_count: 1,
    upcoming_show_count: upcoming,
    total_show_count: upcoming,
    latitude: 33.4,
    longitude: -112.1,
  }
}

describe('pickDriftScene', () => {
  it('returns null for an empty list', () => {
    expect(pickDriftScene([], undefined, () => 0)).toBeNull()
  })

  it('returns null when the only scene is excluded (no in-place "flight")', () => {
    expect(pickDriftScene([scene('phoenix-az', 3)], 'phoenix-az', () => 0)).toBeNull()
  })

  it('never returns the excluded scene', () => {
    const scenes = [scene('a', 5), scene('b', 5)]
    for (const r of [0, 0.25, 0.5, 0.75, 0.999]) {
      expect(pickDriftScene(scenes, 'a', () => r)?.slug).toBe('b')
    }
  })

  it('weights by upcoming_show_count + 1', () => {
    // Weights: a=1 (0 shows), b=10 (9 shows) → total 11. The cumulative line
    // is [0,1) → a, [1,11) → b.
    const scenes = [scene('a', 0), scene('b', 9)]
    expect(pickDriftScene(scenes, undefined, () => 0)?.slug).toBe('a')
    expect(pickDriftScene(scenes, undefined, () => 0.9 / 11)?.slug).toBe('a')
    expect(pickDriftScene(scenes, undefined, () => 1.1 / 11)?.slug).toBe('b')
    expect(pickDriftScene(scenes, undefined, () => 0.999)?.slug).toBe('b')
  })

  it('a zero-show scene stays reachable (weight floor of 1)', () => {
    const scenes = [scene('dead', 0), scene('alive', 100)]
    expect(pickDriftScene(scenes, undefined, () => 0)?.slug).toBe('dead')
  })

  it('guards non-finite counts to the weight floor', () => {
    const scenes = [scene('nan', Number.NaN), scene('b', 1)]
    // NaN weight would poison the cumulative sum; the floor keeps it at 1.
    expect(pickDriftScene(scenes, undefined, () => 0)?.slug).toBe('nan')
    expect(pickDriftScene(scenes, undefined, () => 0.999)?.slug).toBe('b')
  })

  it('returns the last candidate on rand() ~ 1 (float edge)', () => {
    const scenes = [scene('a', 1), scene('b', 1)]
    expect(
      pickDriftScene(scenes, undefined, () => 0.9999999999)?.slug,
    ).toBe('b')
  })
})
