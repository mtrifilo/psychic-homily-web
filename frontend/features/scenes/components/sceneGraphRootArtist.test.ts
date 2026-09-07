import { describe, expect, it } from 'vitest'

import { pickMostBookedSceneArtistSlug } from './sceneGraphRootArtist'

interface TestNode {
  slug: string
  name: string
  upcoming_show_count: number
  entity_type?: string
}

function node(overrides: Partial<TestNode> = {}): TestNode {
  return { slug: 'a-band', name: 'A Band', upcoming_show_count: 1, ...overrides }
}

describe('pickMostBookedSceneArtistSlug', () => {
  it('picks the most upcoming shows', () => {
    const picked = pickMostBookedSceneArtistSlug([
      node({ slug: 'one', name: 'One', upcoming_show_count: 1 }),
      node({ slug: 'four', name: 'Four', upcoming_show_count: 4 }),
      node({ slug: 'two', name: 'Two', upcoming_show_count: 2 }),
    ])
    expect(picked).toBe('four')
  })

  it('breaks a tie by name', () => {
    const picked = pickMostBookedSceneArtistSlug([
      node({ slug: 'zebra', name: 'Zebra', upcoming_show_count: 3 }),
      node({ slug: 'aardvark', name: 'Aardvark', upcoming_show_count: 3 }),
      node({ slug: 'mongoose', name: 'Mongoose', upcoming_show_count: 3 }),
    ])
    expect(picked).toBe('aardvark')
  })

  it('is null when nothing on the canvas has an upcoming show', () => {
    expect(
      pickMostBookedSceneArtistSlug([
        node({ slug: 'one', upcoming_show_count: 0 }),
        node({ slug: 'two', upcoming_show_count: 0 }),
      ]),
    ).toBeNull()
  })

  it.each([null, undefined, [] as TestNode[]])('is null for %p', nodes => {
    expect(pickMostBookedSceneArtistSlug(nodes)).toBeNull()
  })

  // An allowlist, not a label denylist: a node kind the backend adds later must
  // not be handed to an artist endpoint that 404s it.
  it.each(['label', 'venue', 'festival', 'crew'])('never picks a %s node', entityType => {
    const picked = pickMostBookedSceneArtistSlug([
      { slug: 'other-kind', name: 'Other Kind', upcoming_show_count: 40, entity_type: entityType },
      node({ slug: 'sundressed', name: 'Sundressed', upcoming_show_count: 1 }),
    ])
    expect(picked).toBe('sundressed')
  })

  // A hub stands in for a roster, not an artist, and the Observatory centres
  // on artists only.
  it('never picks a label hub, however busy its roster', () => {
    const picked = pickMostBookedSceneArtistSlug([
      node({ slug: 'twelve-xu', name: '12XU', upcoming_show_count: 40, entity_type: 'label' }),
      node({ slug: 'sundressed', name: 'Sundressed', upcoming_show_count: 1 }),
    ])
    expect(picked).toBe('sundressed')
  })

  // A slugless href resolves to the artists INDEX rather than 404, so a
  // slugless node is not a candidate at any activity level.
  it('skips a node with no slug', () => {
    const picked = pickMostBookedSceneArtistSlug([
      node({ slug: '', name: 'Unnamed', upcoming_show_count: 9 }),
      node({ slug: 'sundressed', name: 'Sundressed', upcoming_show_count: 1 }),
    ])
    expect(picked).toBe('sundressed')
  })

  // Payloads served before label hubs shipped carry no discriminator.
  it('treats a node with no entity_type as an artist', () => {
    expect(pickMostBookedSceneArtistSlug([node({ slug: 'legacy', name: 'Legacy' })])).toBe(
      'legacy',
    )
  })
})
