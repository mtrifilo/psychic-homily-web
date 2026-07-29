'use client'

import { useLayoutEffect, useState } from 'react'
import { consumePendingReplay } from '@/lib/hydration/clickReplay'

/**
 * `useLayoutEffect` is the whole point of this hook — see the exactly-once
 * notes in `lib/hydration/clickReplay.ts`. Layout effects flush inside the
 * hydration commit, so no user click can slip between React making the node
 * live and us disarming the capture buffer; a passive effect leaves a
 * frame-wide gap in which a click would fire natively *and* be replayed.
 *
 * React logs a warning if `useLayoutEffect` runs during the server render, so
 * pick the hook at module scope on the presence of a document. This is the
 * same guard Radix ships in `@radix-ui/react-use-layout-effect`, which is
 * already in the tree — it is not a render-time `typeof window` branch, and it
 * cannot produce divergent markup, because neither branch renders anything.
 */
const useIsomorphicLayoutEffect: typeof useLayoutEffect =
  typeof globalThis !== 'undefined' && globalThis.document ? useLayoutEffect : () => {}

/**
 * Replay a click that landed on this control before React hydrated it.
 *
 * Returns a callback ref to attach to the element that also carries
 * `{...replayOnHydrate}`. Everything clicked *inside* that element is covered,
 * so a group of buttons needs one replay root, not one per button:
 *
 * ```tsx
 * const replayRef = useReplayOnHydrate<HTMLDivElement>()
 * <div ref={replayRef} {...replayOnHydrate} role="radiogroup">
 *   <button onClick={…}>Grid</button>
 *   <button onClick={…}>List</button>
 * </div>
 * ```
 *
 * A **callback ref feeding state** is deliberate, not incidental. Several
 * adopters render behind `next/dynamic`, whose first client render is the
 * loading fallback — an effect keyed on anything but the node itself reads a
 * null ref, bails, and never re-runs, silently killing the feature on exactly
 * the late-hydrating hosts that need it most (PSY-1548).
 */
export function useReplayOnHydrate<T extends HTMLElement = HTMLElement>(): (
  node: T | null
) => void {
  const [node, setNode] = useState<T | null>(null)

  useIsomorphicLayoutEffect(() => {
    if (!node) return
    consumePendingReplay(node)
  }, [node])

  return setNode
}
