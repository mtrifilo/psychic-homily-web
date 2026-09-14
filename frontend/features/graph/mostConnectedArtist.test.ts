import { describe, expect, it } from 'vitest'

import { pickMostConnectedArtistSlug } from './mostConnectedArtist'

interface TestNode {
  id: number
  slug: string
  name: string
  entity_type?: string
}

interface TestEdge {
  source_id: number
  target_id: number
}

function node(overrides: Partial<TestNode> = {}): TestNode {
  return { id: 1, slug: 'a-band', name: 'A Band', ...overrides }
}

describe('pickMostConnectedArtistSlug', () => {
  it('picks the artist with the most edges', () => {
    const nodes = [
      node({ id: 1, slug: 'one', name: 'One' }),
      node({ id: 2, slug: 'two', name: 'Two' }),
      node({ id: 3, slug: 'three', name: 'Three' }),
    ]
    const links: TestEdge[] = [
      { source_id: 2, target_id: 1 },
      { source_id: 2, target_id: 3 },
      { source_id: 3, target_id: 2 },
    ]
    // Degrees: two 3, three 2, one 1.
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('two')
  })

  it('counts an edge for the node at either end', () => {
    const nodes = [
      node({ id: 1, slug: 'source-only', name: 'Source Only' }),
      node({ id: 2, slug: 'target-only', name: 'Target Only' }),
      node({ id: 3, slug: 'unlinked', name: 'Aaa Unlinked' }),
    ]
    const links: TestEdge[] = [{ source_id: 1, target_id: 2 }]
    // Both endpoints outrank the unlinked node, whose name sorts first.
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('source-only')
  })

  it('breaks a degree tie by name', () => {
    const nodes = [
      node({ id: 1, slug: 'zebra', name: 'Zebra' }),
      node({ id: 2, slug: 'aardvark', name: 'Aardvark' }),
      node({ id: 3, slug: 'mongoose', name: 'Mongoose' }),
    ]
    const links: TestEdge[] = [
      { source_id: 1, target_id: 2 },
      { source_id: 2, target_id: 3 },
      { source_id: 3, target_id: 1 },
    ]
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('aardvark')
  })

  // The order the payload lists nodes in must not decide the pick.
  it('is order independent', () => {
    const nodes = [
      node({ id: 1, slug: 'zebra', name: 'Zebra' }),
      node({ id: 2, slug: 'aardvark', name: 'Aardvark' }),
    ]
    const links: TestEdge[] = [{ source_id: 1, target_id: 2 }]
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('aardvark')
    expect(pickMostConnectedArtistSlug([...nodes].reverse(), links)).toBe('aardvark')
  })

  it.each([null, undefined, [] as TestNode[]])('is null for %p nodes', nodes => {
    expect(pickMostConnectedArtistSlug(nodes, [])).toBeNull()
  })

  // A canvas of isolates still names a neighbourhood worth opening on.
  it.each([null, undefined, [] as TestEdge[]])(
    'falls back to the alphabetically first artist for %p links',
    links => {
      const nodes = [
        node({ id: 1, slug: 'zebra', name: 'Zebra' }),
        node({ id: 2, slug: 'aardvark', name: 'Aardvark' }),
      ]
      expect(pickMostConnectedArtistSlug(nodes, links)).toBe('aardvark')
    },
  )

  // An allowlist, not a denylist: a node kind a payload gains later must not be
  // handed to an artist endpoint that 404s it.
  it.each(['label', 'venue', 'show', 'release', 'festival'])(
    'never picks a %s node, however connected',
    entityType => {
      const nodes = [
        node({ id: 1, slug: 'other-kind', name: 'Other Kind', entity_type: entityType }),
        node({ id: 2, slug: 'sundressed', name: 'Sundressed', entity_type: 'artist' }),
        node({ id: 3, slug: 'gatecreeper', name: 'Gatecreeper', entity_type: 'artist' }),
      ]
      const links: TestEdge[] = [
        { source_id: 1, target_id: 2 },
        { source_id: 1, target_id: 3 },
        { source_id: 2, target_id: 3 },
        { source_id: 3, target_id: 1 },
      ]
      expect(pickMostConnectedArtistSlug(nodes, links)).toBe('gatecreeper')
    },
  )

  // Station and venue payloads carry no discriminator at all.
  it('treats a node with no entity_type as an artist', () => {
    expect(pickMostConnectedArtistSlug([node({ slug: 'legacy', name: 'Legacy' })], [])).toBe(
      'legacy',
    )
  })

  // The link falls through to the next artist rather than to nothing, so one
  // unusable slug at the TOP of the ranking does not cost the surface its deep
  // link.
  it.each(['', 'Capitalised', 'has space', 'under_score', '../../auth/profile'])(
    'skips a node whose slug the Observatory would refuse (%p)',
    slug => {
      const nodes = [
        node({ id: 1, slug, name: 'Unusable' }),
        node({ id: 2, slug: 'sundressed', name: 'Sundressed' }),
        node({ id: 3, slug: 'zed', name: 'Zed' }),
      ]
      // Degrees: the unusable node 2, the other two 1 each.
      const links: TestEdge[] = [
        { source_id: 1, target_id: 2 },
        { source_id: 1, target_id: 3 },
      ]
      expect(pickMostConnectedArtistSlug(nodes, links)).toBe('sundressed')
    },
  )

  // Same-named artists exist in this catalog, so the name alone is not a total
  // order and payload order must not be allowed to decide the link.
  it('breaks a name tie by slug', () => {
    const nodes = [
      node({ id: 1, slug: 'mirage-tx', name: 'Mirage' }),
      node({ id: 2, slug: 'mirage-az', name: 'Mirage' }),
    ]
    const links: TestEdge[] = [{ source_id: 1, target_id: 2 }]
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('mirage-az')
    expect(pickMostConnectedArtistSlug([...nodes].reverse(), links)).toBe('mirage-az')
  })

  // Collection payloads are mixed-type: an artist's edges to venues, shows and
  // releases count, because the rule ranks connectedness on the drawn canvas.
  it('counts an artist edge to a non-artist node', () => {
    const nodes = [
      node({ id: 1, slug: 'zebra', name: 'Zebra', entity_type: 'artist' }),
      node({ id: 2, slug: 'aardvark', name: 'Aardvark', entity_type: 'artist' }),
      node({ id: 3, slug: 'valley-bar', name: 'Valley Bar', entity_type: 'venue' }),
    ]
    const links: TestEdge[] = [
      { source_id: 1, target_id: 3 },
      { source_id: 1, target_id: 2 },
    ]
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('zebra')
  })

  it('is null when no node carries a usable slug', () => {
    const nodes = [
      node({ id: 1, slug: '', name: 'Unnamed' }),
      node({ id: 2, slug: 'Bad Slug', name: 'Bad' }),
    ]
    expect(pickMostConnectedArtistSlug(nodes, [{ source_id: 1, target_id: 2 }])).toBeNull()
  })

  // An edge naming a node the payload does not draw is ignored rather than
  // counted against some other node.
  it('ignores an edge that names a node outside the payload', () => {
    const nodes = [
      node({ id: 1, slug: 'zebra', name: 'Zebra' }),
      node({ id: 2, slug: 'aardvark', name: 'Aardvark' }),
    ]
    const links: TestEdge[] = [
      { source_id: 1, target_id: 99 },
      { source_id: 1, target_id: 98 },
      { source_id: 2, target_id: 97 },
    ]
    expect(pickMostConnectedArtistSlug(nodes, links)).toBe('zebra')
  })
})
