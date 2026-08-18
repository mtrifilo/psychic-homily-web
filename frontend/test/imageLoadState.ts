/**
 * Image load-state stub (PSY-1719) — extracted from the per-file copies in the
 * ShowHeader / LibraryWallGrid / CollectionCoverImage tests.
 *
 * Several components read an `<img>`'s OWN state on mount, because `onError`
 * cannot see a failure that happened before React attached it (PSY-1685).
 * jsdom never loads anything, so it reports `complete: false` and
 * `naturalWidth: 0` forever — which is "still in flight", not "already
 * failed". Testing either branch means stubbing the pair together:
 *
 *   - `{ complete: true, naturalWidth: 0 }` — finished, decoded nothing. The
 *     dead-hotlink case the mount-time check exists for.
 *   - `{ complete: true, naturalWidth: 600 }` — finished and decoded. Pins the
 *     other half of the predicate, so the check cannot later be loosened to a
 *     bare `naturalWidth === 0` and blank every cached image.
 *
 * `restore()` is not optional and is why this is a helper rather than two
 * inline `vi.spyOn` calls: `test/setup.ts` runs `vi.clearAllMocks()` in
 * `afterEach`, NOT `vi.restoreAllMocks()`. A prototype getter spy therefore
 * survives the test that installed it, and one forgotten restore leaves every
 * later test in the file looking at a permanently broken image.
 *
 * Usage:
 *   const img = stubImageLoadState({ complete: true, naturalWidth: 0 })
 *   try {
 *     render(<Thing />)
 *     expect(screen.getByTestId('fallback')).toBeInTheDocument()
 *   } finally {
 *     img.restore()
 *   }
 */

import { vi } from 'vitest'

interface ImageLoadStateControls {
  /** Restore the real `complete` / `naturalWidth` getters. Always call this. */
  restore: () => void
}

export function stubImageLoadState({
  complete,
  naturalWidth,
}: {
  /** What `HTMLImageElement.prototype.complete` reports. */
  complete: boolean
  /** What `HTMLImageElement.prototype.naturalWidth` reports. */
  naturalWidth: number
}): ImageLoadStateControls {
  const completeSpy = vi
    .spyOn(HTMLImageElement.prototype, 'complete', 'get')
    .mockReturnValue(complete)
  const naturalWidthSpy = vi
    .spyOn(HTMLImageElement.prototype, 'naturalWidth', 'get')
    .mockReturnValue(naturalWidth)

  return {
    restore: () => {
      completeSpy.mockRestore()
      naturalWidthSpy.mockRestore()
    },
  }
}
