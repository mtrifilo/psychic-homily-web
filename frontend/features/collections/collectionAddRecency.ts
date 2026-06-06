/**
 * Client-side "recently added to" signal for the AddToCollectionButton
 * popover (PSY-960 / PSY-893 D3).
 *
 * The data model carries no add-recency: a `Collection` has only
 * `created_at` / `updated_at`, and `updated_at` bumps on ANY edit (title,
 * description, items), so it can't answer "which collections did I recently
 * ADD to". Rather than add a backend column for a sorting nicety, we record
 * the actual add action client-side — the standard pattern for "recently
 * used" menus (Mozilla AwesomeBar, Raycast `useFrecencySorting`, VS Code).
 *
 * Trade-offs, accepted by design: the signal is per-device and is lost on
 * cache-clear / private browsing. That's fine here because it only drives a
 * PROMOTION above an always-present full list — a cold or empty signal simply
 * degrades to the existing flat list (see AddToCollectionButton).
 *
 * Semantics: the stamp is "this COLLECTION was recently added to" (any entity),
 * not "this entity is in this collection". So removing an entity from a
 * collection does NOT un-stamp it — the user may have recently added other
 * things there. A stale stamp self-corrects: adding to other collections
 * pushes it out of the (capped) promoted group.
 */

const STORAGE_KEY = 'psy:collection-add-recency'

// Bound the stored blob: keep only the most-recent N collections' timestamps.
// Far more than the popover ever promotes (5); the cap just stops unbounded
// growth for a heavy curator over months.
const MAX_ENTRIES = 50

/** collectionId (stringified) → epoch ms of the last add to that collection. */
export type CollectionAddRecency = Record<string, number>

/**
 * Read the recency map from localStorage. Always returns a valid object —
 * SSR (no `window`), a missing key, malformed JSON, or non-numeric values
 * all degrade to `{}` rather than throwing.
 */
export function readCollectionAddRecency(): CollectionAddRecency {
  if (typeof window === 'undefined') return {}
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    const out: CollectionAddRecency = {}
    for (const [id, ts] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof ts === 'number' && Number.isFinite(ts)) out[id] = ts
    }
    return out
  } catch {
    return {}
  }
}

/**
 * Record that the user just added the current entity to `collectionId`,
 * stamping `now` (injectable for tests). Prunes to the most-recent
 * MAX_ENTRIES. Best-effort: localStorage failures (quota, disabled,
 * private mode) are swallowed so they can never break the add itself.
 */
export function recordCollectionAdd(
  collectionId: string | number,
  now: number = Date.now()
): void {
  if (typeof window === 'undefined') return
  try {
    const id = String(collectionId)
    const map = readCollectionAddRecency()
    map[id] = now

    let next = map
    if (Object.keys(map).length > MAX_ENTRIES) {
      // Prune to the newest MAX_ENTRIES, but ALWAYS keep the id we just
      // recorded — otherwise a same-/older-timestamp tie at the cap could
      // drop the very thing the user just did. Keep `id` + the newest others.
      const others = Object.entries(map)
        .filter(([k]) => k !== id)
        .sort((a, b) => b[1] - a[1])
        .slice(0, MAX_ENTRIES - 1)
      next = Object.fromEntries([[id, now], ...others])
    }

    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    // Recency is a best-effort nicety — never let a storage error surface.
  }
}
