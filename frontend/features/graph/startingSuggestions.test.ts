import { describe, expect, it } from 'vitest'

import type { GraphStartingPoint } from './hooks/useGraphStartingPoints'
import {
  pickRotationSuggestions,
  SUGGESTION_ROTATION_SIZE,
} from './startingSuggestions'

function pool(size: number): GraphStartingPoint[] {
  return Array.from({ length: size }, (_, i) => ({
    artist_id: i + 1,
    artist_name: `Band ${i + 1}`,
    artist_slug: `band-${i + 1}`,
  }))
}

describe('pickRotationSuggestions', () => {
  it('draws the rotation size from a larger pool', () => {
    const picked = pickRotationSuggestions(pool(12), 1)

    expect(picked).toHaveLength(SUGGESTION_ROTATION_SIZE)
    // The crossfade stacks every offered name in one grid cell, so the pool
    // must NOT all be offered at once — that is what the size cap protects.
    expect(picked.length).toBeLessThan(12)
  })

  it('offers everything it has when the pool is smaller than the rotation', () => {
    const picked = pickRotationSuggestions(pool(2), 7)

    expect(picked.map(p => p.artist_id).sort()).toEqual([1, 2])
  })

  // THE BUG THIS FEATURE EXISTS TO FIX. The old list was fixed and ordered, so
  // the first name a visitor saw was the same every time. Different seeds must
  // reach different names, or the pool is decoration.
  it('varies the draw across seeds', () => {
    const firstNames = new Set(
      Array.from({ length: 40 }, (_, seed) => pickRotationSuggestions(pool(12), seed)[0].artist_id),
    )

    expect(firstNames.size).toBeGreaterThan(1)
  })

  it('is a pure function of pool and seed', () => {
    const candidates = pool(12)

    expect(pickRotationSuggestions(candidates, 4242)).toEqual(
      pickRotationSuggestions(candidates, 4242),
    )
  })

  it('never draws the same artist twice in one rotation', () => {
    for (let seed = 0; seed < 60; seed += 1) {
      const ids = pickRotationSuggestions(pool(4), seed).map(p => p.artist_id)
      expect(new Set(ids).size).toBe(ids.length)
    }
  })

  it('collapses a duplicated id rather than offering it twice', () => {
    const duplicated = [...pool(1), ...pool(1)]

    expect(pickRotationSuggestions(duplicated, 3)).toHaveLength(1)
  })

  // Network input. An entry missing an id, a name, or a slug would render a
  // button announcing "Search for undefined" and centre nothing.
  it('drops entries the catalog cannot honour', () => {
    const malformed = [
      { artist_id: 0, artist_name: 'No Id', artist_slug: 'no-id' },
      { artist_id: 2, artist_name: '', artist_slug: 'no-name' },
      { artist_id: 3, artist_name: 'No Slug', artist_slug: '' },
      null,
      undefined,
      { artist_id: 5, artist_name: 'Fine', artist_slug: 'fine' },
    ] as unknown as GraphStartingPoint[]

    expect(pickRotationSuggestions(malformed, 1)).toEqual([
      { artist_id: 5, artist_name: 'Fine', artist_slug: 'fine' },
    ])
  })

  it('returns nothing for an empty pool', () => {
    expect(pickRotationSuggestions([], 1)).toEqual([])
  })
})
