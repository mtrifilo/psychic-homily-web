import { describe, it, expect, beforeEach } from 'vitest'
import {
  readCollectionAddRecency,
  recordCollectionAdd,
} from './collectionAddRecency'

const KEY = 'psy:collection-add-recency'

describe('collectionAddRecency', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('returns {} when nothing is stored', () => {
    expect(readCollectionAddRecency()).toEqual({})
  })

  it('records an add with the given timestamp and reads it back', () => {
    recordCollectionAdd(42, 1000)
    expect(readCollectionAddRecency()).toEqual({ '42': 1000 })
  })

  it('coerces numeric ids to string keys', () => {
    recordCollectionAdd(7, 500)
    expect(readCollectionAddRecency()).toEqual({ '7': 500 })
  })

  it('overwrites the timestamp on a repeat add', () => {
    recordCollectionAdd(42, 1000)
    recordCollectionAdd(42, 2000)
    expect(readCollectionAddRecency()).toEqual({ '42': 2000 })
  })

  it('degrades to {} on malformed JSON instead of throwing', () => {
    window.localStorage.setItem(KEY, '{not valid json')
    expect(readCollectionAddRecency()).toEqual({})
  })

  it('ignores non-numeric / non-finite values', () => {
    window.localStorage.setItem(
      KEY,
      JSON.stringify({ a: 5, b: 'nope', c: null, d: NaN })
    )
    // NaN serializes to null, so only `a` survives.
    expect(readCollectionAddRecency()).toEqual({ a: 5 })
  })

  it('prunes to the most-recent 50 entries (oldest dropped)', () => {
    // ts = i, so higher i == more recent.
    for (let i = 0; i < 60; i++) recordCollectionAdd(i, i)
    const map = readCollectionAddRecency()
    expect(Object.keys(map)).toHaveLength(50)
    // Newest survive (id 59 = ts 59, id 10 = ts 10); oldest 10 are pruned.
    expect(map['59']).toBe(59)
    expect(map['10']).toBe(10)
    expect(map['9']).toBeUndefined()
    expect(map['0']).toBeUndefined()
  })

  it('keeps all 50 at the boundary (no prune until OVER the cap)', () => {
    for (let i = 0; i < 50; i++) recordCollectionAdd(i, i)
    expect(Object.keys(readCollectionAddRecency())).toHaveLength(50)
  })

  it('never prunes the just-recorded entry, even on a timestamp tie at the cap', () => {
    // 50 collections all stamped at the same instant…
    for (let i = 0; i < 50; i++) recordCollectionAdd(i, 1000)
    // …then a 51st at the SAME (tied) timestamp. The naive "sort + slice top
    // 50" would have dropped it; it must survive.
    recordCollectionAdd(999, 1000)
    const map = readCollectionAddRecency()
    expect(Object.keys(map)).toHaveLength(50)
    expect(map['999']).toBe(1000)
  })
})
