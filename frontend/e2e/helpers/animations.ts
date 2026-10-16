import { expect, type Locator } from '@playwright/test'

/**
 * How long one in-page read waits on the element's animations before it lists
 * them again, so a wait that runs out reports what is still running.
 */
const SETTLE_READ_MS = 1_000

/**
 * Waits in the page, for at most `readMs`, for every animation `el` lists to
 * finish (a cancelled or replaced one counts as finished), then names what is
 * running now: a CSS transition by its property, any other animation by its
 * class. The fallback timer is cleared whichever side wins.
 *
 * Playwright serializes this function into the page, so its body references
 * nothing outside itself.
 */
export async function readAnimationsAfterSettle(el: Element, readMs: number): Promise<string[]> {
  let timer: ReturnType<typeof setTimeout> | undefined
  await Promise.race([
    Promise.allSettled(el.getAnimations().map((a) => a.finished)),
    new Promise((resolve) => {
      timer = setTimeout(resolve, readMs)
    }),
  ])
  clearTimeout(timer)
  return el
    .getAnimations()
    .map((a) => ('transitionProperty' in a ? String(a.transitionProperty) : a.constructor.name))
}

/**
 * Resolves once the element lists no running animation, failing after
 * `budgetMs` with what still runs. `getAnimations()` flushes style first, so a
 * transition that a committed attribute or style starts is listed until it
 * finishes. Each read ends on the frame the animations do rather than a poll
 * interval later.
 *
 * Under SwiftShader an animation advances only when Chromium draws a frame,
 * and a map redraw can hold off frames for seconds, so a caller on /atlas
 * budgets for that rather than for the animation's own duration.
 */
export async function waitForAnimationsSettled(
  locator: Locator,
  { budgetMs, label = String(locator) }: { budgetMs: number; label?: string },
) {
  await expect(async () => {
    const running = await locator.evaluate(readAnimationsAfterSettle, SETTLE_READ_MS)
    expect(running, `${label}: animations still running`).toEqual([])
  }).toPass({ timeout: budgetMs, intervals: [0] })
}
