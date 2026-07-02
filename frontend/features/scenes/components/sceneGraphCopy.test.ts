import { describe, it, expect } from 'vitest'
import type { SceneGraphInfo } from '../types'
import { sceneArtistCountPhrase } from './sceneGraphCopy'

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
  })
})
