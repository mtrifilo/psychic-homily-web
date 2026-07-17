import { describe, it, expect } from 'vitest'
import {
  MAX_TRAIL_SLOTS,
  TRAIL_COLLAPSE_MIN_ENTRIES,
  collapseTrail,
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

describe('shared graphTraversalHistory (PSY-361 trail reducer)', () => {
  describe('pushTrail', () => {
    it('appends to an empty trail', () => {
      expect(pushTrail([], ENTRY(1))).toEqual([ENTRY(1)])
    })

    it('appends to a non-empty trail when distinct', () => {
      expect(pushTrail([ENTRY(1)], ENTRY(2))).toEqual([ENTRY(1), ENTRY(2)])
    })

    it('caps the trail at MAX_TRAIL_SLOTS by dropping the oldest entry', () => {
      // PSY-1474 F3: the middle-collapse display supersedes the original
      // 3-chip render decision; the cap is now a memory bound, and must sit
      // above the collapse threshold so the disclosure is reachable.
      expect(MAX_TRAIL_SLOTS).toBe(10)
      expect(MAX_TRAIL_SLOTS).toBeGreaterThanOrEqual(TRAIL_COLLAPSE_MIN_ENTRIES)

      const trail = Array.from({ length: MAX_TRAIL_SLOTS }, (_, i) => ENTRY(i + 1))
      const next = pushTrail(trail, ENTRY(MAX_TRAIL_SLOTS + 1))
      expect(next).toHaveLength(MAX_TRAIL_SLOTS)
      // Oldest dropped, newest is the tail.
      expect(next[0].id).toBe(2)
      expect(next[next.length - 1].id).toBe(MAX_TRAIL_SLOTS + 1)
    })

    it('continues to drop oldest on each subsequent overflow push', () => {
      let trail: TraversalEntry[] = []
      for (let i = 1; i <= MAX_TRAIL_SLOTS + 3; i++) {
        trail = pushTrail(trail, ENTRY(i))
      }
      expect(trail.map(e => e.id)).toEqual(
        Array.from({ length: MAX_TRAIL_SLOTS }, (_, i) => i + 4),
      )
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

  describe('collapseTrail (PSY-1474 F3 display segments)', () => {
    it('renders flat below the collapse threshold', () => {
      const trail = [ENTRY(1), ENTRY(2), ENTRY(3)]
      expect(collapseTrail(trail)).toEqual({
        leading: trail.map((entry, index) => ({ entry, index })),
        hidden: [],
        trailing: [],
      })
    })

    it('collapses the middle at the threshold, preserving original indices', () => {
      const trail = [ENTRY(1), ENTRY(2), ENTRY(3), ENTRY(4)]
      expect(collapseTrail(trail)).toEqual({
        leading: [{ entry: ENTRY(1), index: 0 }],
        hidden: [
          { entry: ENTRY(2), index: 1 },
          { entry: ENTRY(3), index: 2 },
        ],
        trailing: [{ entry: ENTRY(4), index: 3 }],
      })
    })

    it('hides everything but first and last on long trails', () => {
      const trail = Array.from({ length: 7 }, (_, i) => ENTRY(i + 1))
      const segments = collapseTrail(trail)
      expect(segments.leading.map(s => s.entry.id)).toEqual([1])
      expect(segments.hidden.map(s => s.entry.id)).toEqual([2, 3, 4, 5, 6])
      expect(segments.trailing.map(s => s.entry.id)).toEqual([7])
    })

    it('handles the empty trail', () => {
      expect(collapseTrail([])).toEqual({ leading: [], hidden: [], trailing: [] })
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
