import { describe, it, expect } from 'vitest'

import {
  isLabelHubNode,
  labelHubHomeCaption,
  LABEL_HUB_ENTITY_TYPE,
  LABEL_HUB_HALF_EXTENT,
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
