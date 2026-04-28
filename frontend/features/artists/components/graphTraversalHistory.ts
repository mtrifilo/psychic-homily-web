/**
 * PSY-361: Traversal history for the click-to-re-center artist graph.
 *
 * Tracks the breadcrumb of *prior* centers as the user clicks through the
 * graph. Pure logic, no React — easy to unit-test.
 *
 * The "current" center is *not* stored in the trail. The trail only contains
 * historical anchors (oldest at index 0, most-recent prior at the tail).
 * UI shows up to MAX_TRAIL_SLOTS chips; once exceeded, the OLDEST entry is
 * dropped first so the most recent context is preserved.
 *
 * Decision: max 3 chips (resolved by user before implementation).
 */

export const MAX_TRAIL_SLOTS = 3

export interface TraversalEntry {
  id: number
  slug: string
  name: string
}

/**
 * Push a new prior center onto the trail.
 *
 * Called when the user re-centers: the *outgoing* center becomes a trail
 * entry. Caps the trail at MAX_TRAIL_SLOTS by dropping the oldest entry.
 * Skips no-op pushes when the same artist is already at the tail (prevents
 * back-and-forth click sequences from polluting the trail with duplicates).
 */
export function pushTrail(
  trail: TraversalEntry[],
  entry: TraversalEntry
): TraversalEntry[] {
  const last = trail[trail.length - 1]
  if (last && last.id === entry.id) {
    return trail
  }
  const next = [...trail, entry]
  if (next.length > MAX_TRAIL_SLOTS) {
    return next.slice(next.length - MAX_TRAIL_SLOTS)
  }
  return next
}

/**
 * Truncate the trail at the given index — used when the user clicks a chip
 * to jump back. Everything from `index` onward is dropped (since the user
 * is re-centering on the chip, that artist becomes the new center and is
 * NOT itself in the trail).
 */
export function truncateTrail(
  trail: TraversalEntry[],
  index: number
): TraversalEntry[] {
  if (index < 0) return []
  return trail.slice(0, index)
}

/**
 * Reset the trail entirely — used by the reset button.
 */
export function resetTrail(): TraversalEntry[] {
  return []
}

/**
 * Format the aria-live announcement for re-centers.
 *
 * Used by screen readers and exported so unit tests can pin the format.
 */
export function buildRecenterAnnouncement(
  centerName: string,
  relatedCount: number
): string {
  const noun = relatedCount === 1 ? 'related artist' : 'related artists'
  return `Graph now centered on ${centerName}. ${relatedCount} ${noun} shown.`
}
