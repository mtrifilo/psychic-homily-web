/**
 * A per-instance memory of keys whose answer was YES, each for a fixed time.
 *
 * Only positives are ever stored, so the only stale answer it can give is "yes"
 * for at most `ttlMs` after the truth changed; a "no" is never remembered and is
 * always asked again.
 *
 * Bounded at `maxEntries`. Every entry lives the same `ttlMs`, so insertion
 * order is expiry order: when full, the entry dropped to make room is the one
 * closest to expiring anyway. A key that cannot be stored costs one more probe
 * later, never a wrong answer.
 */
export interface PositiveVerdictCache {
  /** True when `key` was remembered as positive within the last `ttlMs`. */
  has(key: string): boolean
  /** Remember `key` as positive for `ttlMs` from now. */
  remember(key: string): void
  /** Forget everything. */
  clear(): void
}

export function createPositiveVerdictCache({
  ttlMs,
  maxEntries,
  now = () => Date.now(),
}: {
  ttlMs: number
  maxEntries: number
  now?: () => number
}): PositiveVerdictCache {
  const expiries = new Map<string, number>()

  return {
    has(key) {
      const expiresAt = expiries.get(key)
      if (expiresAt === undefined) return false
      if (expiresAt > now()) return true
      expiries.delete(key)
      return false
    },
    remember(key) {
      // Deleted first so a refreshed key moves to the back of the order.
      expiries.delete(key)
      if (expiries.size >= maxEntries) {
        const oldest = expiries.keys().next()
        if (!oldest.done) expiries.delete(oldest.value)
      }
      expiries.set(key, now() + ttlMs)
    },
    clear() {
      expiries.clear()
    },
  }
}
