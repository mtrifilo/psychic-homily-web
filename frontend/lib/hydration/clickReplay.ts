/**
 * Capture-and-replay for clicks that land before React hydrates a control.
 *
 * ## Why this exists
 *
 * A server-rendered control is painted, focusable and clickable well before
 * React attaches its handlers. A click in that window does nothing: no error,
 * no feedback, and **React 19 does not replay it**. PSY-1610 proved the drop —
 * 39 verified pre-hydration clicks produced 0 effects, 42 after produced 42 —
 * and measured the window from ~260ms on loopback to ~6.7s on a throttled
 * phone. The measured table lives in `docs/performance-improvements.md`; it is
 * deliberately not duplicated here, so there is one copy to re-measure.
 *
 * **Visible does not imply interactive.** Anything in server HTML has this
 * window, including anonymous surfaces, so it is not an authentication
 * concern. Navigation is the one thing that survives for free, because a real
 * `<a href>` is handled by the browser rather than by React — which is why
 * links must never be an `onClick` router push.
 *
 * ## How it works
 *
 * 1. `CLICK_REPLAY_SCRIPT` runs inline in `<head>`, before any framework code,
 *    recording the pointer/click sequence that lands inside any element marked
 *    `data-replay-on-hydrate` and not yet flagged as hydrated.
 * 2. Adopting components spread {@link replayOnHydrate}. Its `ref` fires when
 *    React attaches the node, at which point {@link consumePendingReplay}
 *    flags the element, takes the buffered sequence, and re-dispatches it at
 *    the original target node.
 *
 * ## Exactly-once
 *
 * A double-fired Save is worse than the bug this fixes, so the ordering inside
 * `consumePendingReplay` is load-bearing:
 *
 * - The element is flagged hydrated **first**, so the capture listener stops
 *   buffering it before anything is dispatched. Replayed events are untrusted
 *   and rejected on that basis too — two independent guards against a
 *   capture/replay loop.
 * - The buffer entry is deleted **before** dispatch, so a second invocation
 *   (StrictMode's double-invoked effects, a remount) finds nothing to replay.
 * - It runs from a **ref callback**, which React invokes during the commit
 *   that makes the node live, synchronously and before paint. No user click
 *   can be processed in between. Moving this to a passive `useEffect` would
 *   open a frame-wide gap in which a real click would both fire natively *and*
 *   be replayed — that is the double fire. **The test suite cannot catch that
 *   regression** (jsdom does not reproduce the gap), so it is stated here.
 *
 * ## Adoption
 *
 * Spread `replayOnHydrate` onto the element that owns the interaction:
 *
 * ```tsx
 * <button {...replayOnHydrate} onClick={…} />
 * ```
 *
 * Everything clicked *inside* a replay root is covered, so a group of buttons
 * needs one root, not one per button. It is a single spreadable object on
 * purpose: an earlier revision needed a `ref` *and* a separate attribute, and
 * attaching only the attribute silently reproduced the original bug on an
 * element that looked adopted.
 *
 * `BracketLink` and the shared bracket/menu controls already carry it, so most
 * new code inherits this without doing anything.
 *
 * ## Known limits
 *
 * - **Click only.** Keyboard activation of a Radix trigger goes through
 *   `onKeyDown`, which is not captured, so a pre-hydration `Enter` on such a
 *   trigger is still dropped.
 * - **Opt-in.** Only marked elements are buffered. A fully automatic version
 *   is not implementable rather than merely blunt: replay needs a *per-node*
 *   "React has attached handlers here" signal, and React exposes none
 *   publicly. An app-level "hydration done" flag fires too early for lazily
 *   hydrated subtrees — page-body controls hydrate after the TopBar — so it
 *   would replay into dead nodes and drop the click a second time.
 * - Capture stops after {@link MAX_REPLAY_AGE_MS}; past that a buffered click
 *   could not be replayed anyway.
 */

/** Marks an element as a replay root. Clicks inside it are buffered pre-hydration. */
export const REPLAY_ATTR = 'data-replay-on-hydrate'

/**
 * How long a captured click stays replayable, and how long the capture
 * listeners stay installed.
 *
 * 10s covers every condition PSY-1610 measured, including the ~6.7s worst case
 * on a 20x-throttled phone over slow 4G — precisely the user this exists for.
 * Past the cutoff the click is dropped silently: surfacing a message about an
 * interaction from more than ten seconds ago would confuse more than explain.
 */
export const MAX_REPLAY_AGE_MS = 10_000

/**
 * The sequence a real mouse click produces, in order.
 *
 * Replaying the whole sequence rather than a lone `click` is required, not
 * belt-and-braces: Radix's `DropdownMenuTrigger` (the TopBar user menu) opens
 * on `onPointerDown`, while its `PopoverTrigger` siblings (notification bell,
 * add-to-collection) open on `onClick`. A click-only replay was measured
 * against the proof harness in `e2e-hydration/` — the save mutation still
 * worked and the user menu stayed shut.
 *
 * Read only by the inline script below.
 */
const REPLAYED_EVENTS = ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'] as const

/**
 * Upper bound on buffered events per interaction.
 *
 * A pointer interaction can only reach five (one of each type, since a new
 * `pointerdown` restarts the entry). The cap exists for keyboard repeat:
 * holding `Enter` on a focused button emits `click` after `click` with no
 * intervening `pointerdown`, so nothing would otherwise reset the list.
 */
const MAX_EVENTS_PER_ENTRY = 8

interface ReplayRecord {
  type: string
  pointer: boolean
  init: Record<string, unknown>
}

interface ReplayEntry {
  /** The deepest element actually clicked — replay targets the node, never a coordinate. */
  target: HTMLElement
  /** `event.timeStamp` of the most recent event in the sequence. */
  time: number
  events: ReplayRecord[]
}

declare global {
  interface Window {
    __phClickReplay?: {
      /**
       * Weakly keyed on purpose: an entry whose root never hydrates is never
       * consumed, and a strong Map on `window` would pin that element — and,
       * through it, its whole detached subtree — for the life of the session,
       * across every client-side navigation.
       */
      buffer: WeakMap<HTMLElement, ReplayEntry>
      /** Removes the capture listeners and drops the buffer. */
      stop: () => void
    }
  }
}

/**
 * The inline pre-hydration capture script, as source text.
 *
 * Dependency-free and ES5-flavoured: it runs before any bundle, so it cannot
 * import anything and must not out-run older parsers. Only `REPLAY_ATTR` is a
 * shared contract with {@link consumePendingReplay}; the other interpolated
 * values are internal to this script.
 */
export const CLICK_REPLAY_SCRIPT = `(function () {
  var ATTR = '${REPLAY_ATTR}';
  var TYPES = ${JSON.stringify(REPLAYED_EVENTS)};
  var MAX = ${MAX_EVENTS_PER_ENTRY};
  var store = { buffer: new WeakMap(), stop: stop };
  window.__phClickReplay = store;

  function snapshot(e) {
    var init = {
      bubbles: true,
      cancelable: e.cancelable,
      composed: true,
      detail: e.detail,
      clientX: e.clientX,
      clientY: e.clientY,
      ctrlKey: e.ctrlKey,
      altKey: e.altKey,
      shiftKey: e.shiftKey,
      metaKey: e.metaKey,
      button: e.button,
      buttons: e.buttons
    };
    var isPointer = typeof PointerEvent === 'function' && e instanceof PointerEvent;
    if (isPointer) {
      init.pointerId = e.pointerId;
      init.pointerType = e.pointerType;
      init.isPrimary = e.isPrimary;
    }
    return { type: e.type, pointer: isPointer, init: init };
  }

  function capture(e) {
    // Untrusted events are our own replays: never re-buffer them.
    if (!e.isTrusted) return;
    var target = e.target;
    if (!target || typeof target.closest !== 'function') return;
    var root = target.closest('[' + ATTR + ']');
    // Once the owning component has hydrated it handles its own clicks.
    if (!root || root.dataset.replayHydrated) return;

    var entry = store.buffer.get(root);
    // A new pointerdown starts a fresh interaction, so only the user's LAST
    // attempt is ever replayed no matter how many times they click.
    if (!entry || e.type === 'pointerdown') {
      entry = { target: target, time: e.timeStamp, events: [] };
      store.buffer.set(root, entry);
    }
    entry.target = target;
    entry.time = e.timeStamp;
    if (entry.events.length < MAX) entry.events.push(snapshot(e));
  }

  function stop() {
    for (var j = 0; j < TYPES.length; j++) {
      document.removeEventListener(TYPES[j], capture, true);
    }
    store.buffer = new WeakMap();
  }

  for (var i = 0; i < TYPES.length; i++) {
    document.addEventListener(TYPES[i], capture, true);
  }

  // Nothing buffered past the staleness cutoff could be replayed anyway, and
  // by then every server-rendered control has long since hydrated. Stopping
  // takes the per-click ancestor walk out of the steady state entirely.
  setTimeout(stop, ${MAX_REPLAY_AGE_MS});
})();`

/**
 * Flag `root` as hydrated and replay any click buffered against it.
 *
 * Exposed for tests and for callers that own an unusual replay root; ordinary
 * components should spread {@link replayOnHydrate} instead. See the
 * exactly-once notes in the module doc comment for why the ordering here and
 * the ref-callback timing both matter.
 *
 * @returns `true` when a buffered interaction was replayed.
 */
export function consumePendingReplay(root: HTMLElement): boolean {
  // First, unconditionally: stop the capture listener buffering this element.
  root.dataset.replayHydrated = '1'

  const store = typeof window === 'undefined' ? undefined : window.__phClickReplay
  if (!store) return false

  // Consume before dispatching, so a second call has nothing left to replay.
  const entry = store.buffer.get(root)
  store.buffer.delete(root)
  if (!entry || entry.events.length === 0) return false

  // A click the user made long enough ago may no longer reflect their intent.
  if (performance.now() - entry.time > MAX_REPLAY_AGE_MS) return false

  // Replay at the node, not at a coordinate: layout shifts as images load, so
  // a recorded position can land somewhere else entirely by now.
  const target = entry.target
  if (!target.isConnected || !root.contains(target)) return false

  for (const record of entry.events) {
    const Ctor =
      record.pointer && typeof PointerEvent === 'function' ? PointerEvent : MouseEvent
    target.dispatchEvent(new Ctor(record.type, record.init))
  }
  return true
}

/**
 * Ref callback that replays anything buffered against this node.
 *
 * Module-level and closes over nothing, so it is referentially stable and
 * costs adopters no state and no extra render — which matters because these
 * controls render per row in lists, and the extra work would land inside the
 * very hydration window this feature exists to compensate for.
 */
function attachReplayRoot(node: HTMLElement | null): void {
  if (node) consumePendingReplay(node)
}

/**
 * Spread onto any control that ships in server HTML.
 *
 * ```tsx
 * <button {...replayOnHydrate} onClick={…} />
 * ```
 *
 * One object rather than a ref plus an attribute, so the two halves cannot be
 * separated — attaching the attribute without the ref silently reproduces the
 * original bug on an element that looks adopted.
 */
export const replayOnHydrate = {
  ref: attachReplayRoot,
  [REPLAY_ATTR]: '',
} as const
