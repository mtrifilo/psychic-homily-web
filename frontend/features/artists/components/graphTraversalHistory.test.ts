import { describe, it, expect } from 'vitest'
import {
  MAX_TRAIL_SLOTS,
  pushTrail,
  truncateTrail,
  resetTrail,
  buildRecenterAnnouncement,
  type TraversalEntry,
} from './graphTraversalHistory'

const ENTRY = (id: number, name = `A${id}`): TraversalEntry => ({
  id,
  slug: `a${id}`,
  name,
})

describe('graphTraversalHistory (PSY-361 trail reducer)', () => {
  describe('pushTrail', () => {
    it('appends to an empty trail', () => {
      expect(pushTrail([], ENTRY(1))).toEqual([ENTRY(1)])
    })

    it('appends to a non-empty trail when distinct', () => {
      expect(pushTrail([ENTRY(1)], ENTRY(2))).toEqual([ENTRY(1), ENTRY(2)])
    })

    it('caps the trail at MAX_TRAIL_SLOTS by dropping the oldest entry', () => {
      // Sanity: the user-resolved decision is 3.
      expect(MAX_TRAIL_SLOTS).toBe(3)

      const trail = [ENTRY(1), ENTRY(2), ENTRY(3)]
      const next = pushTrail(trail, ENTRY(4))
      expect(next).toHaveLength(3)
      // Oldest dropped, newest is the tail.
      expect(next.map(e => e.id)).toEqual([2, 3, 4])
    })

    it('continues to drop oldest on each subsequent overflow push', () => {
      let trail: TraversalEntry[] = []
      for (let i = 1; i <= 6; i++) {
        trail = pushTrail(trail, ENTRY(i))
      }
      expect(trail.map(e => e.id)).toEqual([4, 5, 6])
    })

    it('skips no-op when the same artist is already the tail', () => {
      // Prevents click-the-same-node twice from polluting the trail.
      const trail = [ENTRY(1), ENTRY(2)]
      expect(pushTrail(trail, ENTRY(2))).toBe(trail)
    })

    it('does NOT skip when the duplicate is mid-trail (only tail-equality skips)', () => {
      // Push of an older entry creates a new trail position — different
      // navigation pattern (re-visiting an artist after detouring), and
      // the trail order is the user's history.
      const trail = [ENTRY(1), ENTRY(2)]
      expect(pushTrail(trail, ENTRY(1))).toEqual([ENTRY(1), ENTRY(2), ENTRY(1)])
    })
  })

  describe('truncateTrail', () => {
    it('drops everything from the given index onward', () => {
      const trail = [ENTRY(1), ENTRY(2), ENTRY(3)]
      expect(truncateTrail(trail, 1)).toEqual([ENTRY(1)])
    })

    it('returns empty when truncating from index 0', () => {
      expect(truncateTrail([ENTRY(1), ENTRY(2)], 0)).toEqual([])
    })

    it('returns empty when index is negative', () => {
      expect(truncateTrail([ENTRY(1)], -1)).toEqual([])
    })
  })

  describe('resetTrail', () => {
    it('returns an empty array', () => {
      expect(resetTrail()).toEqual([])
    })
  })

  describe('buildRecenterAnnouncement (aria-live string)', () => {
    it('singularizes when there is exactly 1 related artist', () => {
      expect(buildRecenterAnnouncement('Gatecreeper', 1)).toBe(
        'Graph now centered on Gatecreeper. 1 related artist shown.'
      )
    })

    it('pluralizes for 0 or many related artists', () => {
      expect(buildRecenterAnnouncement('Gatecreeper', 0)).toBe(
        'Graph now centered on Gatecreeper. 0 related artists shown.'
      )
      expect(buildRecenterAnnouncement('Frozen Soul', 12)).toBe(
        'Graph now centered on Frozen Soul. 12 related artists shown.'
      )
    })
  })
})
