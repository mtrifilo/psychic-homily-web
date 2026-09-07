import { describe, expect, it } from 'vitest'

import { GRAPH_PATH, GRAPH_ROOT_PARAM, graphRootHref } from './graphRootLink'

describe('graphRootHref', () => {
  it('roots the map on a slug', () => {
    expect(graphRootHref('sundressed')).toBe('/graph?artist=sundressed')
  })

  it('encodes a slug so a stray character cannot open a second param', () => {
    expect(graphRootHref('a&b=c')).toBe('/graph?artist=a%26b%3Dc')
  })

  // An empty slug is reachable: entity slugs are nullable in this schema, and
  // `?artist=` names nothing the Observatory can resolve.
  it.each([undefined, null, ''])('falls back to the plain map for %p', slug => {
    expect(graphRootHref(slug)).toBe(GRAPH_PATH)
  })

  it('names the param the Observatory reads', () => {
    expect(GRAPH_ROOT_PARAM).toBe('artist')
  })
})
