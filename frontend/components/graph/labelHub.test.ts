import { describe, it, expect } from 'vitest'

import {
  isLabelHubNode,
  labelHubHomeCaption,
  LABEL_HUB_ENTITY_TYPE,
  LABEL_HUB_HALF_EXTENT,
  LABEL_HUB_SPOKE_EDGE_TYPE,
  spokeRestLength,
  SPOKE_REST_LENGTH_MAX,
  SPOKE_REST_LENGTH_MIN,
} from './labelHub'

describe('isLabelHubNode', () => {
  it('is true only for the label entity type', () => {
    expect(isLabelHubNode({ entity_type: LABEL_HUB_ENTITY_TYPE })).toBe(true)
    expect(isLabelHubNode({ entity_type: 'artist' })).toBe(false)
  })

  // Payloads served before hubs shipped carry no entity_type; those nodes are
  // artists and must keep rendering as circles.
  it('treats a missing entity_type as an artist', () => {
    expect(isLabelHubNode({})).toBe(false)
    expect(isLabelHubNode({ entity_type: undefined })).toBe(false)
  })

  it('does not match other entity types', () => {
    for (const type of ['venue', 'release', 'show', 'festival', 'Label']) {
      expect(isLabelHubNode({ entity_type: type })).toBe(false)
    }
  })
})

describe('labelHubHomeCaption', () => {
  it('captions a US label as city + state, without the redundant country', () => {
    expect(
      labelHubHomeCaption({ city: 'Austin', state: 'TX', country: 'US' }),
    ).toBe('Austin, TX')
  })

  it('keeps the country for an international label', () => {
    expect(
      labelHubHomeCaption({ city: 'London', country: 'England' }),
    ).toBe('London, England')
  })

  // Sacred Bones is the live case: country on file, no city/state. A country
  // alone is still useful ("this anchor is not local"), so it captions.
  it('falls back to country alone when city and state are unknown', () => {
    expect(labelHubHomeCaption({ country: 'US' })).toBe('US')
  })

  // A named PSY-1792 acceptance case, and the one with a non-obvious output: a
  // BARE state. The US-country suppression keys off the state alone, not off
  // "city AND state", so a label placed only in a state captions "TX" rather
  // than "TX, USA". Pinned because a hub captioned with two letters looks like
  // a bug to anyone who has not read `formatLocation`.
  it('falls back to state alone, still without the redundant US country', () => {
    expect(labelHubHomeCaption({ state: 'TX', country: 'USA' })).toBe('TX')
    // Non-US keeps the country, which is what makes the state legible.
    expect(labelHubHomeCaption({ state: 'Ontario', country: 'Canada' })).toBe(
      'Ontario, Canada',
    )
  })

  it('returns undefined — not a placeholder — when nothing is on file', () => {
    expect(labelHubHomeCaption({})).toBeUndefined()
    expect(
      labelHubHomeCaption({ city: '', state: '   ', country: '' }),
    ).toBeUndefined()
  })
})

describe('LABEL_HUB_HALF_EXTENT', () => {
  // A hub anchors a whole roster, so it must read as larger than the artist
  // circles (NODE_RADIUS 8) it gathers.
  it('is larger than the artist node radius', () => {
    expect(LABEL_HUB_HALF_EXTENT).toBeGreaterThan(8)
  })
})

describe('spokeRestLength', () => {
  // d3's ~30px default packs a roster onto a ring too small for its own
  // labels — the crowding hubs exist to remove.
  it('is well above the d3 default link distance', () => {
    expect(spokeRestLength(3)).toBeGreaterThan(30)
  })

  it('grows with roster size, since ring circumference does', () => {
    expect(spokeRestLength(25)).toBeGreaterThan(spokeRestLength(6))
  })

  it('clamps both ends so small rosters stay compact and big ones stay on screen', () => {
    expect(spokeRestLength(1)).toBe(SPOKE_REST_LENGTH_MIN)
    expect(spokeRestLength(500)).toBe(SPOKE_REST_LENGTH_MAX)
    expect(spokeRestLength(0)).toBe(SPOKE_REST_LENGTH_MIN)
  })
})

describe('LABEL_HUB_SPOKE_EDGE_TYPE', () => {
  it('matches the backend on_label edge type', () => {
    expect(LABEL_HUB_SPOKE_EDGE_TYPE).toBe('on_label')
  })
})
