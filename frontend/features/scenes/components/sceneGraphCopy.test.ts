import { describe, it, expect } from 'vitest'
import type { SceneGraphInfo } from '../types'
import { sceneArtistCountPhrase, sceneIsolateHookCopy, sceneLabelCountPhrase } from './sceneGraphCopy'

function scene(overrides: Partial<SceneGraphInfo>): SceneGraphInfo {
  return {
    slug: 'phoenix-az',
    city: 'Phoenix',
    state: 'AZ',
    artist_count: 12,
    edge_count: 4,
    metro_roster_total: 12,
    roster_truncated: false,
    ...overrides,
  }
}

describe('sceneArtistCountPhrase (PSY-1296)', () => {
  it('renders the plain count when the roster is not truncated', () => {
    expect(sceneArtistCountPhrase(scene({}))).toBe('12 artists')
  })

  it('singularizes a one-artist roster', () => {
    expect(
      sceneArtistCountPhrase(
        scene({ artist_count: 1, metro_roster_total: 1 }),
      ),
    ).toBe('1 artist')
  })

  it('renders the truncation hint when the cap bit', () => {
    expect(
      sceneArtistCountPhrase(
        scene({ metro_roster_total: 90, roster_truncated: true }),
      ),
    ).toBe('top 12 of 90 artists')
  })

  it('degrades to the plain count when the flag is set but the total is nonsense', () => {
    // A skewed/stale payload must never render "showing top 12 of 0 artists".
    expect(
      sceneArtistCountPhrase(
        scene({ metro_roster_total: 0, roster_truncated: true }),
      ),
    ).toBe('12 artists')
    expect(
      sceneArtistCountPhrase(
        scene({ metro_roster_total: 12, roster_truncated: true }),
      ),
    ).toBe('12 artists')
    // A missing total (older payload at runtime) must degrade too — the TS
    // type says number, but the wire isn't validated.
    expect(
      sceneArtistCountPhrase(
        scene({
          metro_roster_total: undefined as unknown as number,
          roster_truncated: true,
        }),
      ),
    ).toBe('12 artists')
  })

  it('degrades to "0 artists" for an empty capped roster, never "top 0 of N" (PSY-1476)', () => {
    // The shared helper's `shown > 0` guard covers scene too: an empty roster
    // flagged as truncated must not render "top 0 of 5 artists".
    expect(
      sceneArtistCountPhrase(
        scene({ artist_count: 0, metro_roster_total: 5, roster_truncated: true }),
      ),
    ).toBe('0 artists')
  })
})

describe('sceneLabelCountPhrase (PSY-1530)', () => {
  const scene = (label_count?: number) => ({
    slug: 'austin-tx',
    city: 'Austin',
    state: 'TX',
    artist_count: 38,
    edge_count: 27,
    metro_roster_total: 38,
    roster_truncated: false,
    label_count,
  })

  it('pluralizes label counts', () => {
    expect(sceneLabelCountPhrase(scene(2))).toBe('2 labels')
    expect(sceneLabelCountPhrase(scene(1))).toBe('1 label')
  })

  // A hub-less scene must read exactly as it did before hubs existed, so the
  // caller has nothing to join.
  it('returns null when the graph has no hubs', () => {
    expect(sceneLabelCountPhrase(scene(0))).toBeNull()
  })

  // Payloads served before hubs shipped omit the field entirely.
  it('returns null when label_count is absent', () => {
    expect(sceneLabelCountPhrase(scene(undefined))).toBeNull()
  })
})

describe('sceneIsolateHookCopy (PSY-1785, locked P2)', () => {
  it('is the locked sentence, verbatim, including the leading plus', () => {
    expect(sceneIsolateHookCopy(10)).toBe(
      '+10 not yet connected artists. Add a show or a label to put one on the map',
    )
    expect(sceneIsolateHookCopy(1)).toBe(
      '+1 not yet connected artists. Add a show or a label to put one on the map',
    )
  })

  it('returns null when there are no isolates', () => {
    expect(sceneIsolateHookCopy(0)).toBeNull()
  })
})
