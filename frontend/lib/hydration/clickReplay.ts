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
 * needs one root, not one per button — but **only when the whole group hydrates
 * in the same commit as the root**. The root is flagged as soon as its own ref
 * fires; if a child hydrates later (a `next/dynamic` or Suspense boundary
 * inside the root), the buffered click is dispatched at a child that is still
 * dead, and the flag then suppresses any further capture for that subtree. When
 * in doubt, put the root on the element that owns the handler.
 *
 * It is a single spreadable object so an ordinary adopter cannot attach one
 * half without the other: an earlier revision needed a `ref` *and* a separate
 * attribute, and attaching only the attribute silently reproduced the original
 * bug on an element that looked adopted. The halves are still *separable* by a
 * component that must compose the ref with a forwarded one — `BracketLink`
 * does exactly that — so in those files the marker attribute is the
 * load-bearing half and must survive any ref refactor. Two things guard that:
 * `BracketLink`'s own tests assert the attribute reaches the DOM, and in
 * development an unflagged replay root logs an error after load.
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
 * - Capture stops after {@link MAX_REPLAY_AGE_MS} so the per-click ancestor walk
 *   leaves the steady state. Anything still buffered stays replayable until the
 *   staleness check rejects it.
 * - A replay is skipped if the target has become disabled since the click, and
 *   an interaction that never produced a click is never replayed at all.
 * - A replay is dropped if the clicked node itself does not survive hydration
 *   (a control that swaps out its children once real state arrives). Falling
 *   back to the replay root was considered and rejected: for a group root it
 *   would dispatch at a node with no handler — a silent no-op reading as
 *   success.
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
 * A real interaction reaches at most five — one of each type — because both a
 * new `pointerdown` and a completed `click` start a fresh entry. This is a
 * pure backstop against an event sequence nobody anticipated; it is not what
 * makes keyboard repeat safe (the post-click reset is).
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
  /**
   * `performance.now()` when the most recent event of the sequence was
   * captured — deliberately NOT `event.timeStamp`.
   *
   * {@link consumePendingReplay} compares this against `performance.now()`, so
   * both sides must share one clock. Real browsers put `event.timeStamp` on the
   * same origin (verified: 0.4ms apart in Chromium), but jsdom does not for
   * constructed events, and `Date.now()` would be ~1.7e12 ms out. Reading the
   * clock directly makes the invariant true by construction everywhere, and the
   * sub-millisecond difference from the true event time is irrelevant against a
   * 10s cutoff. Do NOT substitute `Date.now()`: it fails silently and in
   * opposite directions — nothing ever stale in capture, everything instantly
   * stale in consume.
   */
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
      /**
       * Removes the capture listeners. Runs automatically once the staleness
       * cutoff has passed; anything already buffered stays replayable.
       */
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
  /* EDITING BELOW: this is a TEMPLATE LITERAL injected via
     dangerouslySetInnerHTML. Three sequences must never appear in it -- a
     backtick and a dollar-brace each break the build (loud, fine), and a
     closing script tag would break out of the element (silent, not fine).
     clickReplay.test.ts asserts the last one. */
  // A browser re-executes an inline script if the node is re-inserted. Without
  // this, a second evaluation would swap in an empty buffer -- discarding every
  // click captured so far -- and leave a duplicate set of listeners behind.
  if (window.__phClickReplay) return;

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
    // Only primary, unmodified interactions. A middle/right button or a
    // cmd/ctrl/shift-click is a browser-owned gesture (open in a new tab, range
    // select) that the browser has already acted on -- replaying it would run
    // the action a second time. React handlers do not drive those gestures.
    if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    var target = e.target;
    if (!target || typeof target.closest !== 'function') return;
    var root = target.closest('[' + ATTR + ']');
    // Once the owning component has hydrated it handles its own clicks.
    if (!root || root.dataset.replayHydrated) return;

    var entry = store.buffer.get(root);
    // Start a fresh interaction on a new pointerdown, AND on the first event
    // after a completed click. Both halves are needed: keyboard activation
    // (Enter/Space on a focused button) emits a bare click with no pointerdown,
    // so without the second condition a user pressing Enter twice -- the
    // natural response to a control that appears dead -- would append a second
    // click and replay TWO mutations.
    // (No backticks in here: this comment lives inside a template literal.)
    var last = entry && entry.events.length
      ? entry.events[entry.events.length - 1].type
      : null;
    // Also start fresh when this event belongs to a different control inside
    // the same root: pressing down on one button, dragging off, then activating
    // a sibling would otherwise splice one gesture onto another and dispatch
    // both at the sibling. Containment is allowed because a genuine click
    // legitimately lands pointerdown on an inner icon and click on the button.
    var sameControl = entry && (entry.target === target
      || entry.target.contains(target) || target.contains(entry.target));
    if (!entry || e.type === 'pointerdown' || last === 'click' || !sameControl) {
      entry = { target: target, time: performance.now(), events: [] };
      store.buffer.set(root, entry);
    }
    entry.target = target;
    entry.time = performance.now();
    if (entry.events.length < MAX) entry.events.push(snapshot(e));
  }

  // Removes the listeners but deliberately KEEPS the buffer. Clearing it would
  // discard a click captured just before the cutoff whose control hydrates just
  // after it — a click younger than the staleness limit, on exactly the very
  // slow device this feature exists for. Nothing leaks by keeping it: the
  // buffer is a WeakMap, and staleness is enforced at consume time anyway.
  function stop() {
    for (var j = 0; j < TYPES.length; j++) {
      document.removeEventListener(TYPES[j], capture, true);
    }
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

  // Never replay into a control the user could no longer click. `dispatchEvent`
  // ignores the disabled attribute — that only blocks *user-initiated*
  // activation — so a control that was enabled in server HTML and disabled by
  // the time it hydrated would otherwise fire its handler anyway. Reachable
  // today: DensityToggle's radios ship enabled (view mode defaults to grid) and
  // become disabled if the persisted preference turns out to be list.
  if (target.closest('button:disabled, input:disabled, [aria-disabled="true"]')) {
    return false
  }

  const types = entry.events.map(record => record.type)

  // Only completed interactions replay. A gesture that produced no click — a
  // touch-scroll that began on a control, a press that dragged off before
  // release, teardown landing mid-gesture — must not be re-enacted: a bare
  // pointerdown is enough to open a Radix DropdownMenu, so a user who merely
  // swiped with their thumb starting on the avatar would find the account menu
  // open by itself at hydration. The whole-sequence replay exists to satisfy
  // pointerdown-opening triggers INSIDE a completed click, not on their own.
  if (types.lastIndexOf('click') === -1) return false

  // Keep at most one record per event type, on the consuming side too — this is
  // the side that actually causes the mutation, so it is worth making
  // unconditional rather than trusting the capture-side reset to stay correct.
  // Per-type rather than click-only: `pointerdown` activates a Radix
  // DropdownMenu, so a duplicate pointerdown toggles a menu open then shut just
  // as surely as a duplicate click double-fires a save. A real interaction has
  // each type once, so this is a no-op in the normal case.
  const records = entry.events.filter(
    (record, i) => types.lastIndexOf(record.type) === i
  )

  for (const record of records) {
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
