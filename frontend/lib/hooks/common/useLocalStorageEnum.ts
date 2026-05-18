'use client'

import { useCallback, useState, useSyncExternalStore } from 'react'

// The 'storage' event only fires in OTHER tabs/windows, never the tab that
// wrote the value. This custom event keeps multiple readers of the same key
// in sync within a single tab.
const SAME_TAB_EVENT = 'ph-local-storage-change'

function subscribe(callback: () => void): () => void {
  window.addEventListener('storage', callback)
  window.addEventListener(SAME_TAB_EVENT, callback)
  return () => {
    window.removeEventListener('storage', callback)
    window.removeEventListener(SAME_TAB_EVENT, callback)
  }
}

/**
 * SSR-safe persisted enum state, backed by useSyncExternalStore.
 *
 * - Server + initial hydration render return `defaultValue` (no SSR mismatch).
 * - After hydration, the client snapshot reads localStorage and re-renders.
 * - Subscribes to `storage` (cross-tab) AND a same-tab custom event so multiple
 *   components reading the same key stay in sync.
 * - Values not in `allowed` (corrupted storage, stale schema) fall back to
 *   `defaultValue`.
 * - When localStorage is unavailable (private mode, disabled, quota) the
 *   per-component intent layer keeps the calling component responsive even
 *   though persistence and cross-component sync are degraded.
 *
 * The `allowed` array should be stable across renders (define as a module-level
 * `as const` tuple, not an inline literal) so the snapshot getter doesn't
 * churn each render.
 */
export function useLocalStorageEnum<T extends string>(
  key: string,
  defaultValue: T,
  allowed: ReadonlyArray<T>
): readonly [T, (next: T) => void] {
  // Per-component intent layer. The intent is tagged with the key it was set
  // for; reads with a different key naturally ignore it (no useEffect-based
  // reset needed when the key prop changes). Cleared during render once
  // storage catches up so cross-tab / cross-component updates can win again.
  const [intent, setIntent] = useState<{ key: string; value: T } | null>(null)

  const getClientSnapshot = useCallback((): T => {
    try {
      const raw = window.localStorage.getItem(key)
      if (raw !== null && (allowed as readonly string[]).includes(raw)) {
        return raw as T
      }
    } catch {
      // localStorage unavailable
    }
    return defaultValue
  }, [key, defaultValue, allowed])

  const getServerSnapshot = useCallback((): T => defaultValue, [defaultValue])

  const storageValue = useSyncExternalStore(
    subscribe,
    getClientSnapshot,
    getServerSnapshot
  )

  const intentApplies = intent !== null && intent.key === key
  const value = intentApplies ? intent.value : storageValue

  // Compare-during-render reset (the React-recommended alternative to a
  // useEffect — see react.dev/learn/you-might-not-need-an-effect): once
  // storage agrees with the intent for the current key, drop the intent so
  // subsequent cross-tab / cross-component updates win.
  if (intentApplies && storageValue === intent.value) {
    setIntent(null)
  }

  const setValue = useCallback(
    (next: T) => {
      setIntent({ key, value: next })
      try {
        window.localStorage.setItem(key, next)
        window.dispatchEvent(new Event(SAME_TAB_EVENT))
      } catch {
        // localStorage unavailable; the keyed intent keeps this component's
        // UI live. Other components reading the same key will not re-render
        // until localStorage recovers — acceptable for the private-mode /
        // quota-exceeded edge cases.
      }
    },
    [key]
  )

  return [value, setValue] as const
}
