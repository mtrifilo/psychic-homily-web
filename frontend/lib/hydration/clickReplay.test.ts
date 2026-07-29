import {
  describe,
  it,
  expect,
  beforeAll,
  afterAll,
  beforeEach,
  afterEach,
  vi,
} from 'vitest'
import {
  CLICK_REPLAY_SCRIPT,
  MAX_REPLAY_AGE_MS,
  REPLAY_ATTR,
  consumePendingReplay,
} from './clickReplay'

/**
 * The capture half of this primitive is an inline script string, so these tests
 * evaluate that exact string rather than a re-implementation — otherwise the
 * thing that actually ships would be untested.
 */
function installCaptureScript() {
  new Function(CLICK_REPLAY_SCRIPT)()
}

/**
 * The capture listener ignores untrusted events on purpose — that is one of its
 * two guards against re-buffering its own replays — but jsdom marks every
 * scripted event untrusted, and does so *during* dispatch, so the flag cannot
 * simply be set beforehand.
 *
 * The way in: capture-phase listeners fire window-first, so this one runs
 * before the primitive's document-level listener and re-marks the event through
 * jsdom's internal impl object while the dispatch is in flight. Only events
 * this file explicitly created as user input are re-marked, so the
 * untrusted-input test still exercises the real guard.
 */
const intendedTrusted = new WeakSet<Event>()

function reTrustDuringDispatch(event: Event) {
  if (!intendedTrusted.has(event)) return
  const impl = Object.getOwnPropertySymbols(event)
    .map(sym => (event as unknown as Record<symbol, unknown>)[sym])
    .find(
      (value): value is { isTrusted: boolean } =>
        typeof value === 'object' && value !== null && 'isTrusted' in value
    )
  if (!impl) {
    throw new Error(
      'jsdom event impl not found — reTrustDuringDispatch() needs updating'
    )
  }
  impl.isTrusted = true
}

const USER_EVENT_TYPES = [
  'pointerdown',
  'mousedown',
  'pointerup',
  'mouseup',
  'click',
] as const

function clickAsUser(
  target: Element,
  type = 'click',
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

/** A replay root wrapping a button, as the adopters render it. */
function mountControl() {
  const root = document.createElement('div')
  root.setAttribute(REPLAY_ATTR, '')
  const button = document.createElement('button')
  root.appendChild(button)
  document.body.appendChild(root)
  return { root, button }
}

describe('pre-hydration click capture and replay', () => {
  // Installed once: the script registers permanent document listeners, so
  // re-running it per test would stack duplicates. Tests get a clean slate by
  // clearing the buffer instead.
  let store: NonNullable<Window['__phClickReplay']>

  beforeAll(() => {
    for (const type of USER_EVENT_TYPES) {
      window.addEventListener(type, reTrustDuringDispatch, true)
    }
    installCaptureScript()
    store = window.__phClickReplay!
  })

  afterAll(() => {
    for (const type of USER_EVENT_TYPES) {
      window.removeEventListener(type, reTrustDuringDispatch, true)
    }
  })

  beforeEach(() => {
    document.body.innerHTML = ''
    // One test deletes the global to prove the consumer is inert without it.
    window.__phClickReplay = store
    store.buffer.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('replays a click that landed before the control hydrated', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()
    button.addEventListener('click', onClick)

    // The user clicks while the page is painted but not yet interactive. React
    // is not listening, so nothing happens — mirrored here by attaching the
    // handler only after the click.
    button.removeEventListener('click', onClick)
    clickAsUser(button)
    expect(onClick).not.toHaveBeenCalled()

    button.addEventListener('click', onClick)
    expect(consumePendingReplay(root)).toBe(true)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('replays exactly once even if hydration runs the consumer twice', () => {
    const { root, button } = mountControl()
    clickAsUser(button)

    const onClick = vi.fn()
    button.addEventListener('click', onClick)

    // StrictMode double-invokes effects; a remount would call this again too.
    expect(consumePendingReplay(root)).toBe(true)
    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('does not buffer clicks that land after the control hydrated', () => {
    const { root, button } = mountControl()

    // Hydration happened first: this is now a live control handling its own
    // clicks, so a later consume must not double-fire them.
    consumePendingReplay(root)

    const onClick = vi.fn()
    button.addEventListener('click', onClick)
    clickAsUser(button)
    expect(onClick).toHaveBeenCalledTimes(1)

    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('never re-buffers its own replayed events', () => {
    const { root, button } = mountControl()
    clickAsUser(button)
    consumePendingReplay(root)

    // The replay dispatched an untrusted click through the same capture
    // listener. Nothing may be left in the buffer for a second round.
    expect(window.__phClickReplay?.buffer.size).toBe(0)
  })

  it('drops a click older than the staleness cutoff', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()
    clickAsUser(button)
    button.addEventListener('click', onClick)

    const captured = window.__phClickReplay!.buffer.get(root)!
    vi.spyOn(performance, 'now').mockReturnValue(
      captured.time + MAX_REPLAY_AGE_MS + 1
    )

    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('still replays a click just inside the staleness cutoff', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()
    clickAsUser(button)
    button.addEventListener('click', onClick)

    const captured = window.__phClickReplay!.buffer.get(root)!
    vi.spyOn(performance, 'now').mockReturnValue(
      captured.time + MAX_REPLAY_AGE_MS - 1
    )

    expect(consumePendingReplay(root)).toBe(true)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('replays the whole pointer sequence in order, not just the click', () => {
    // Radix's DropdownMenuTrigger opens on pointerdown, so a click-only replay
    // would leave the user menu shut.
    const { root, button } = mountControl()
    const seen: string[] = []

    clickAsUser(button, 'pointerdown')
    clickAsUser(button, 'mousedown')
    clickAsUser(button, 'mouseup')
    clickAsUser(button)

    for (const type of ['pointerdown', 'mousedown', 'mouseup', 'click']) {
      button.addEventListener(type, () => seen.push(type))
    }

    consumePendingReplay(root)
    expect(seen).toEqual(['pointerdown', 'mousedown', 'mouseup', 'click'])
  })

  it('replays only the last interaction when the user clicks repeatedly', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()

    clickAsUser(button, 'pointerdown')
    clickAsUser(button)
    clickAsUser(button, 'pointerdown')
    clickAsUser(button)

    button.addEventListener('click', onClick)
    consumePendingReplay(root)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('targets the clicked node rather than a coordinate', () => {
    // Layout shifts as images load, so a recorded position can point somewhere
    // else entirely by the time hydration finishes.
    const { root, button } = mountControl()
    const sibling = document.createElement('button')
    root.appendChild(sibling)

    const onTarget = vi.fn()
    const onSibling = vi.fn()

    clickAsUser(button, 'click', { clientX: 10, clientY: 10 })

    button.addEventListener('click', onTarget)
    sibling.addEventListener('click', onSibling)

    // Move the original target somewhere else entirely.
    button.getBoundingClientRect = () =>
      ({ left: 999, top: 999, width: 10, height: 10 }) as DOMRect

    consumePendingReplay(root)
    expect(onTarget).toHaveBeenCalledTimes(1)
    expect(onSibling).not.toHaveBeenCalled()
  })

  it('drops the replay when the captured target left the DOM', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()
    clickAsUser(button)
    button.addEventListener('click', onClick)

    button.remove()

    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('ignores clicks outside any replay root', () => {
    const loose = document.createElement('button')
    document.body.appendChild(loose)

    clickAsUser(loose)
    expect(window.__phClickReplay?.buffer.size).toBe(0)
  })

  it('ignores untrusted clicks, so scripted input is never queued', () => {
    const { root, button } = mountControl()
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(window.__phClickReplay?.buffer.size).toBe(0)
    expect(consumePendingReplay(root)).toBe(false)
  })

  it('is inert when the capture script never ran', () => {
    const { root } = mountControl()
    delete window.__phClickReplay

    expect(() => consumePendingReplay(root)).not.toThrow()
    expect(root.dataset.replayHydrated).toBe('1')
  })
})
