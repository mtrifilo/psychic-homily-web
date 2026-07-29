/**
 * Capture-and-replay for clicks that land before React hydrates a control.
 *
 * ## Why this exists
 *
 * A server-rendered control is painted, focusable and clickable well before
 * React attaches its handlers. A click in that window does nothing: no error,
 * no feedback, and **React 19 does not replay it**. PSY-1610 measured the gap
 * on a production build (median of 3, FCP -> that node interactive):
 *
 * | condition                | dead window |
 * | ------------------------ | ----------- |
 * | loopback, 1x CPU         | ~260 ms     |
 * | 4x CPU                   | ~505 ms     |
 * | 6x CPU + slow 4G         | ~4.6 s      |
 * | 20x CPU + slow 4G        | ~6.7 s      |
 *
 * and proved the drop: 39 verified-on-target clicks before the node hydrated
 * produced 0 effects; 42 after produced 42. No overlap.
 *
 * **Visible does not imply interactive.** Anything that ships in server HTML
 * has this window — including anonymous surfaces, so this is not an
 * authentication concern. Navigation is the one case that survives for free,
 * because a real `<a href>` is handled by the browser rather than by React:
 * that is why links must never be an `onClick` router push.
 *
 * ## How it works
 *
 * 1. `CLICK_REPLAY_SCRIPT` runs inline in `<head>`, before any framework code.
 *    It records the pointer/click sequence landing inside any element marked
 *    `data-replay-on-hydrate` that has not yet been flagged as hydrated.
 * 2. When the owning component hydrates, {@link consumePendingReplay} flags the
 *    element, takes the buffered sequence, and re-dispatches it at the original
 *    target node.
 *
 * ## Exactly-once
 *
 * A double-fired Save is worse than the bug this fixes, so the ordering inside
 * `consumePendingReplay` is load-bearing and deliberate:
 *
 * - The element is flagged hydrated **first**, so the capture listener stops
 *   buffering it before anything is dispatched. The replayed events are
 *   untrusted anyway and are rejected on that basis too — two independent
 *   guards against a capture/replay loop.
 * - The buffer entry is deleted **before** dispatch, so a second invocation
 *   (StrictMode's double-invoked effects, a remount) finds nothing to replay.
 * - Callers run this from a *layout* effect. Layout effects flush synchronously
 *   within the hydration commit, so no user click can be processed between the
 *   moment React makes the node live and the moment we stop buffering it. With
 *   a passive `useEffect` that gap is a whole frame, and a real click landing
 *   in it would both work natively *and* be replayed — a double fire.
 *
 * ## Known limits
 *
 * - **Click only.** Keyboard activation of a Radix trigger goes through
 *   `onKeyDown`, which is not captured. A pre-hydration `Enter` on such a
 *   trigger is still dropped.
 * - **Opt-in.** Only marked elements are buffered; replaying every button in
 *   the document would be far too blunt a hammer.
 * - The captured sequence is limited to the event types in `REPLAYED_EVENTS`.
 *   A control that opens on some *other* event will not replay correctly — the
 *   sequence is faithful to what the browser dispatches for a real click, not
 *   to arbitrary custom gestures.
 */

/** Marks an element as a replay root. Clicks inside it are buffered pre-hydration. */
export const REPLAY_ATTR = 'data-replay-on-hydrate'

/**
 * Spread onto the JSX element that owns a replay root, so adopters can't
 * misspell the attribute:
 *
 * ```tsx
 * <button ref={replayRef} {...replayOnHydrate} onClick={…} />
 * ```
 */
export const replayOnHydrate = { [REPLAY_ATTR]: '' } as const

/**
 * How long a captured click stays replayable.
 *
 * 10s covers every condition PSY-1610 measured, including the 6.7s worst case
 * on a 20x-throttled phone over slow 4G — which is precisely the user this
 * exists for. Past the cutoff the click is dropped silently: surfacing a
 * message about an interaction from more than ten seconds ago would confuse
 * more than it explains.
 */
export const MAX_REPLAY_AGE_MS = 10_000

/**
 * The sequence a real mouse click produces, in order. Replaying the whole
 * sequence rather than a lone `click` is required, not belt-and-braces:
 * Radix's `DropdownMenuTrigger` (the TopBar user menu) opens on
 * `onPointerDown`, so a bare `click` would leave it shut, while its
 * `PopoverTrigger` siblings (notification bell, add-to-collection) open on
 * `onClick`. Replaying what the browser actually sends satisfies both without
 * per-component special-casing.
 */
const REPLAYED_EVENTS = ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'] as const

/** Upper bound on buffered events per interaction — a guard, never reached in practice. */
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
    __phClickReplay?: { buffer: Map<HTMLElement, ReplayEntry> }
  }
}

/**
 * The inline pre-hydration capture script, as source text.
 *
 * Deliberately dependency-free and written in ES5 style: it runs before any
 * bundle, so it cannot import anything and must not out-run older parsers.
 * Kept in sync with {@link consumePendingReplay} by the shared constants
 * interpolated below.
 */
export const CLICK_REPLAY_SCRIPT = `(function () {
  var ATTR = '${REPLAY_ATTR}';
  var TYPES = ${JSON.stringify(REPLAYED_EVENTS)};
  var MAX = ${MAX_EVENTS_PER_ENTRY};
  var buffer = new Map();
  window.__phClickReplay = { buffer: buffer };

  function snapshot(e) {
    var init = {
      bubbles: true,
      cancelable: e.cancelable,
      composed: true,
      // \`view\` is deliberately not carried over: nothing in the replayed set
      // reads it, and passing a Window across the snapshot is the one field
      // that behaves differently under jsdom than in a browser.
      detail: e.detail,
      screenX: e.screenX,
      screenY: e.screenY,
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

    var entry = buffer.get(root);
    // A new pointerdown starts a fresh interaction, so only the user's LAST
    // attempt is ever replayed no matter how many times they click.
    if (!entry || e.type === 'pointerdown') {
      entry = { target: target, time: e.timeStamp, events: [] };
      buffer.set(root, entry);
    }
    entry.target = target;
    entry.time = e.timeStamp;
    if (entry.events.length < MAX) entry.events.push(snapshot(e));
  }

  for (var i = 0; i < TYPES.length; i++) {
    document.addEventListener(TYPES[i], capture, true);
  }
})();`

/**
 * Flag `root` as hydrated and replay any click buffered against it.
 *
 * Call from a layout effect once the owning component is interactive — see the
 * exactly-once notes in the module doc comment for why the ordering here and
 * the effect timing both matter.
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
