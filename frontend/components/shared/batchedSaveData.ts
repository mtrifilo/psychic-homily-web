/**
 * The contract between a list that fetches save counts in ONE batched request
 * and the per-item save buttons it renders.
 *
 * # Why this is a union rather than a plain optional object
 *
 * The buttons fall back to their own per-item request when a parent doesn't
 * supply save data. Deciding that from `saveData === undefined` alone is wrong,
 * because undefined means two different things:
 *
 *   1. "Nobody is fetching this for me"      → self-fetch is correct
 *   2. "The batch owns this and is IN FLIGHT" → self-fetch is a bug
 *
 * Case 2 is invisible on a fast connection and severe on a slow one: every card
 * fires its own request racing the batch that exists to replace them. A 50-show
 * list became ~50 requests per load, which is what tripped the public-read rate
 * limit for a real user browsing on cellular — where the batch's latency, and so
 * the race window, is widest.
 *
 * Encoding "pending" in the value itself makes the inconsistent state
 * unrepresentable, rather than relying on callers to pass a second boolean prop
 * that can drift out of sync with the data one.
 */
export type SaveCounts = { save_count: number; is_saved: boolean }

/**
 * What a batching parent hands down. `'pending'` means a batched request owns
 * this item and has not resolved yet — the button must render its empty state
 * and must NOT issue a per-item request.
 */
export type BatchedSaveData = SaveCounts | 'pending'

/**
 * Build the prop for one item from a batch query's result map.
 *
 * `map` is undefined while the batch query is pending OR disabled (React Query
 * reports a disabled query as pending), so both windows correctly suppress
 * self-fetching. A disabled batch means its own gate isn't satisfied yet —
 * typically the viewer's id is still resolving — and the per-item queries are
 * gated on exactly the same thing, so nothing is lost by waiting.
 *
 * If the batch HAS resolved and simply has no entry for this id, the result is
 * `undefined` and the button self-fetches. That is a genuine fallback rather
 * than a race: the backend seeds an entry for every requested id, so it should
 * not happen, and one stray request is the right cost for being wrong.
 */
export function batchedSaveFor(
  map: Record<string, SaveCounts> | undefined,
  id: number | string
): BatchedSaveData | undefined {
  if (!map) return 'pending'
  return map[String(id)]
}

/**
 * Split the prop into the value to render and whether the button owns fetching.
 * Kept next to the type so both save-button variants resolve it identically.
 */
export function resolveBatchedSaveData(saveData?: BatchedSaveData): {
  value: SaveCounts | undefined
  shouldSelfFetch: boolean
} {
  if (saveData === 'pending') return { value: undefined, shouldSelfFetch: false }
  return { value: saveData, shouldSelfFetch: saveData === undefined }
}
