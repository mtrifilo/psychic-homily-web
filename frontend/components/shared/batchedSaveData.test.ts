import { describe, it, expect } from 'vitest'
import {
  batchedSaveFor,
  resolveBatchedSaveData,
  type SaveCounts,
} from './batchedSaveData'

// `shouldSelfFetch` decides whether a save button issues its OWN request, so it
// is what stands between a disabled batch query and one request per visible row.
// The batch reads are disabled while auth is unsettled (see AuthStatus in
// lib/context/AuthContext), which makes that window the widest it has ever been,
// and every list surface routes through here.
describe('resolveBatchedSaveData', () => {
  const counts: SaveCounts = { save_count: 4, is_saved: true }

  it('self-fetches only when nobody is fetching for this row', () => {
    expect(resolveBatchedSaveData(undefined)).toEqual({
      value: undefined,
      shouldSelfFetch: true,
    })
  })

  it('suppresses the self-fetch while a batch owns the row', () => {
    expect(resolveBatchedSaveData('pending')).toEqual({
      value: undefined,
      shouldSelfFetch: false,
    })
  })

  it('suppresses the self-fetch when the batch supplied a value', () => {
    expect(resolveBatchedSaveData(counts)).toEqual({
      value: counts,
      shouldSelfFetch: false,
    })
  })

  // The chart call sites and ReleaseList do not use `batchedSaveFor`: they pass
  // `data?.[id] ?? { save_count: n, is_saved: false }`, so a disabled or pending
  // batch hands this a TRUTHY zeroed object rather than undefined. That shape
  // must suppress the self-fetch too — a falsy-check here would turn one
  // batch request into one per row on the highest-traffic pages.
  it('treats a zeroed fallback as owned, not as absent', () => {
    const zeroed: SaveCounts = { save_count: 0, is_saved: false }
    expect(resolveBatchedSaveData(zeroed)).toEqual({
      value: zeroed,
      shouldSelfFetch: false,
    })
  })
})

describe('batchedSaveFor', () => {
  // A disabled TanStack query reports `data: undefined`, exactly as a pending
  // one does, so this is the window the batch guards widened.
  it('reports pending while the batch map is absent', () => {
    expect(batchedSaveFor(undefined, 7)).toBe('pending')
  })

  it('returns the row a resolved batch supplied, keyed by string id', () => {
    const counts: SaveCounts = { save_count: 2, is_saved: false }
    expect(batchedSaveFor({ '7': counts }, 7)).toBe(counts)
  })

  // A resolved batch missing this id is a genuine fallback, not a race: the
  // backend seeds an entry per requested id, so one stray request is the right
  // cost for being wrong.
  it('falls through to a self-fetch when a resolved batch omits the row', () => {
    expect(batchedSaveFor({ '7': { save_count: 2, is_saved: false } }, 9)).toBe(
      undefined
    )
  })
})
