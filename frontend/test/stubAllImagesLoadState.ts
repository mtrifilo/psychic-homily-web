/**
 * Image load-state stub (PSY-1719) — extracted from the per-file copies in the
 * ShowHeader / LibraryWallGrid / CollectionCoverImage tests.
 *
 * **This affects EVERY `<img>` rendered while it is installed.** It spies on
 * `HTMLImageElement.prototype`, so there is no per-element control: a test
 * rendering a subject image beside an avatar or a logo stubs all of them, and
 * two calls in one test do not give two different images two different
 * states. If you ever need per-element control, `Object.defineProperty` on
 * the specific node is the escape hatch — don't reach for a second call here.
 *
 * Why it exists: several components read an `<img>`'s OWN state on mount,
 * because `onError` cannot see a failure that happened before React attached
 * it (PSY-1685). jsdom never loads anything, so it reports `complete: false`
 * and `naturalWidth: 0` forever — which is "still in flight", not "already
 * failed". Testing either branch means stubbing the pair together:
 *
 *   - `{ complete: true, naturalWidth: 0 }` — finished, decoded nothing. The
 *     dead-hotlink case the mount-time check exists for.
 *   - `{ complete: true, naturalWidth: 600 }` — finished and decoded. Pins the
 *     other half of the predicate, which nothing else reaches: jsdom's
 *     `complete` is false for an http src, so the unstubbed tests never
 *     exercise it.
 *
 * Teardown is automatic via `onTestFinished`, and that matters:
 * `test/setup.ts` runs `vi.clearAllMocks()` in `afterEach`, NOT
 * `vi.restoreAllMocks()`, and `vitest.config.mts` sets no `restoreMocks`. A
 * prototype getter spy would therefore survive the test that installed it and
 * leave every later test in the file looking at a permanently broken image.
 * The returned `restore` is only for un-stubbing partway through a test.
 *
 * Usage:
 *   stubAllImagesLoadState({ complete: true, naturalWidth: 0 })
 *   render(<Thing />)
 *   expect(screen.getByTestId('fallback')).toBeInTheDocument()
 */

import { onTestFinished, vi } from 'vitest'

interface ImageLoadStateControls {
  /**
   * Restore the real `complete` / `naturalWidth` getters early. Optional —
   * teardown is already registered for the end of the test.
   */
  restore: () => void
}

export function stubAllImagesLoadState({
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

  const restore = () => {
    completeSpy.mockRestore()
    naturalWidthSpy.mockRestore()
  }

  onTestFinished(restore)

  return { restore }
}
