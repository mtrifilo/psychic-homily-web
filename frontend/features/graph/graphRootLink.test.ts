import { describe, expect, it } from 'vitest'

import { graphRootHref } from './graphRootLink'

describe('graphRootHref', () => {
  it('roots the map on a slug', () => {
    expect(graphRootHref('sundressed')).toBe('/graph?artist=sundressed')
  })

  // The reader refuses anything that is not slug-shaped, so the writer must
  // too: a link that looks rooted and silently lands on the overview is worse
  // than one that never claimed to be.
  it.each(['a&b=c', '../../auth/profile', 'Bad Slug', 'Diners'])(
    'refuses a value that is not slug-shaped (%s)',
    value => {
      expect(graphRootHref(value)).toBe('/graph')
    },
  )

  // An empty slug is reachable: entity slugs are nullable in this schema, and
  // `?artist=` names nothing the Observatory can resolve.
  it.each([undefined, null, ''])('falls back to the plain map for %p', slug => {
    expect(graphRootHref(slug)).toBe('/graph')
  })
})
