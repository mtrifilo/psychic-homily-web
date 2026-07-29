/**
 * Test-only helper for driving the click-replay primitive with events it will
 * accept as real user input.
 *
 * The capture listener ignores untrusted events on purpose — that is one of
 * its two guards against re-buffering its own replays — but jsdom marks every
 * scripted event untrusted, and does so *during* dispatch, so the flag cannot
 * simply be set beforehand.
 *
 * The way in: capture-phase listeners fire window-first, so the bridge
 * installed here runs before the primitive's document-level listener and
 * re-marks the event through jsdom's internal impl object while the dispatch
 * is still in flight.
 *
 * Shared between the unit tests and the hydration tests deliberately: this is
 * a walk into jsdom internals with a real failure mode, and it should have one
 * place to fix rather than two copies drifting apart.
 */

const intendedTrusted = new WeakSet<Event>()

/**
 * Every type the primitive listens for. The bridge must cover all of them —
 * an earlier copy of this helper registered only `pointerdown` and `click`,
 * which quietly made any test using `mousedown` assert nothing.
 */
const USER_EVENT_TYPES = [
  'pointerdown',
  'mousedown',
  'pointerup',
  'mouseup',
  'click',
] as const

function reTrustDuringDispatch(event: Event): void {
  if (!intendedTrusted.has(event)) return
  const impl = Object.getOwnPropertySymbols(event)
    .map(sym => (event as unknown as Record<symbol, unknown>)[sym])
    .find(
      (value): value is { isTrusted: boolean } =>
        typeof value === 'object' && value !== null && 'isTrusted' in value
    )
  // Throw rather than degrade quietly: a silently-untrusted event would make
  // most of the calling suite assert nothing at all.
  if (!impl) {
    throw new Error('jsdom event impl not found — trustedEvents.ts needs updating')
  }
  impl.isTrusted = true
}

/** Install the bridge. Returns its teardown, for `afterAll`. */
export function installTrustedEventBridge(): () => void {
  for (const type of USER_EVENT_TYPES) {
    window.addEventListener(type, reTrustDuringDispatch, true)
  }
  return () => {
    for (const type of USER_EVENT_TYPES) {
      window.removeEventListener(type, reTrustDuringDispatch, true)
    }
  }
}

/** Dispatch one event at `target` that the primitive will treat as real input. */
export function clickAsUser(
  target: Element,
  type: string = 'click',
  init: MouseEventInit = {}
): MouseEvent {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    button: 0,
    ...init,
  })
  intendedTrusted.add(event)
  target.dispatchEvent(event)
  return event
}
