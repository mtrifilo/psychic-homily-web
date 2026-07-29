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
import { installTrustedEventBridge, clickAsUser } from './testing/trustedEvents'

/**
 * The capture half of this primitive is an inline script string, so these
 * tests evaluate that exact string rather than a re-implementation —
 * otherwise the thing that actually ships would be untested.
 */
function installCaptureScript() {
  new Function(CLICK_REPLAY_SCRIPT)()
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
  // re-running it per test would stack duplicates. Each test builds fresh
  // elements, and the buffer is weakly keyed, so nothing leaks between them.
  let store: NonNullable<Window['__phClickReplay']>
  let teardownBridge: () => void

  beforeAll(() => {
    teardownBridge = installTrustedEventBridge()
    installCaptureScript()
    store = window.__phClickReplay!
  })

  afterAll(() => {
    teardownBridge()
  })

  beforeEach(() => {
    document.body.innerHTML = ''
    // One test deletes the global to prove the consumer is inert without it.
    window.__phClickReplay = store
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('replays a click that landed before the control hydrated', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()

    // The user clicks while the page is painted but not yet interactive.
    // React is not listening, mirrored here by attaching the handler after.
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

    // The replay dispatched untrusted events through the same capture
    // listener. Nothing may be left for a second round.
    expect(store.buffer.get(root)).toBeUndefined()
  })

  it('drops a click older than the staleness cutoff', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()
    clickAsUser(button)
    button.addEventListener('click', onClick)

    const captured = store.buffer.get(root)!
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

    const captured = store.buffer.get(root)!
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

  it('replays exactly once when keyboard activation repeats with no pointerdown', () => {
    // Enter/Space on a focused button emits a bare `click` — no pointerdown to
    // start a new interaction. A user who presses Enter twice because nothing
    // appeared to happen (the whole premise of this bug) must still get ONE
    // mutation, not two. An earlier revision replayed every buffered click.
    const { root, button } = mountControl()
    const onClick = vi.fn()

    clickAsUser(button)
    clickAsUser(button)
    clickAsUser(button)

    button.addEventListener('click', onClick)
    expect(consumePendingReplay(root)).toBe(true)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('replays exactly once when a mouse click is followed by keyboard activation', () => {
    // Mixed input: the full pointer sequence, then a bare keyboard click.
    const { root, button } = mountControl()
    const onClick = vi.fn()

    clickAsUser(button, 'pointerdown')
    clickAsUser(button, 'mousedown')
    clickAsUser(button, 'mouseup')
    clickAsUser(button)
    clickAsUser(button)

    button.addEventListener('click', onClick)
    consumePendingReplay(root)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('never buffers more than one click per interaction (capture-side guard)', () => {
    // Pins the capture half of the exactly-once defence specifically. Without
    // its own assertion, removing this guard leaves the suite green because the
    // consume-side filter independently covers the behaviour.
    const { root, button } = mountControl()
    clickAsUser(button)
    clickAsUser(button)
    clickAsUser(button)

    const events = store.buffer.get(root)!.events
    expect(events.filter(e => e.type === 'click')).toHaveLength(1)
  })

  it('dispatches at most one click even if the buffer holds several (consume-side guard)', () => {
    // Pins the consume half. Builds the invalid state directly, since the
    // capture guard now prevents it from arising naturally.
    const { root, button } = mountControl()
    clickAsUser(button)

    const entry = store.buffer.get(root)!
    const clickRecord = entry.events.find(e => e.type === 'click')!
    entry.events = [clickRecord, clickRecord, clickRecord]

    const onClick = vi.fn()
    button.addEventListener('click', onClick)
    expect(consumePendingReplay(root)).toBe(true)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('keeps the buffer at one entry-worth under sustained keyboard repeat', () => {
    // Asserting the per-entry CAP here would be vacuous now that a completed
    // click resets the entry — 40 clicks leave exactly one record, which is the
    // property actually worth pinning.
    const { root, button } = mountControl()
    for (let i = 0; i < 40; i++) clickAsUser(button)

    expect(store.buffer.get(root)!.events).toHaveLength(1)
  })

  it('never lets a full mouse interaction exceed one record per type', () => {
    const { root, button } = mountControl()
    clickAsUser(button, 'pointerdown')
    clickAsUser(button, 'mousedown')
    clickAsUser(button, 'pointerup')
    clickAsUser(button, 'mouseup')
    clickAsUser(button)

    const events = store.buffer.get(root)!.events
    expect(events).toHaveLength(5)
    expect(new Set(events.map(e => e.type)).size).toBe(5)
  })

  it('captures a timestamp on performance.now()\'s origin, not Date.now()', () => {
    // The staleness check compares entry.time against performance.now().
    // Swapping either side to Date.now() fails silently and in opposite
    // directions, and no other test would notice.
    const { root, button } = mountControl()
    clickAsUser(button)

    const entry = store.buffer.get(root)!
    expect(Math.abs(performance.now() - entry.time)).toBeLessThan(5_000)
  })

  it('never replays an interaction that produced no click', () => {
    // A touch-scroll that begins on a control emits pointerdown and then
    // pointercancel — never a click. Replaying the bare pointerdown would open
    // a Radix DropdownMenu the user never asked for, at hydration.
    const { root, button } = mountControl()
    const onPointerDown = vi.fn()

    clickAsUser(button, 'pointerdown')
    clickAsUser(button, 'mousedown')

    button.addEventListener('pointerdown', onPointerDown)
    expect(consumePendingReplay(root)).toBe(false)
    expect(onPointerDown).not.toHaveBeenCalled()
  })

  it('ignores non-primary buttons', () => {
    // Middle/right-click are browser-owned gestures, not React interactions.
    const { root, button } = mountControl()
    clickAsUser(button, 'pointerdown', { button: 1 })
    clickAsUser(button, 'click', { button: 1 })

    expect(store.buffer.get(root)).toBeUndefined()
    expect(consumePendingReplay(root)).toBe(false)
  })

  it('ignores modifier clicks, which the browser has already acted on', () => {
    const { root, button } = mountControl()
    clickAsUser(button, 'click', { metaKey: true })
    clickAsUser(button, 'click', { shiftKey: true })
    clickAsUser(button, 'click', { ctrlKey: true })

    expect(store.buffer.get(root)).toBeUndefined()
    expect(consumePendingReplay(root)).toBe(false)
  })

  it('does not splice a gesture on one control onto a sibling', () => {
    // Press down on A, drag off without releasing, then activate sibling B by
    // keyboard. B must not receive A's pointerdown.
    const root = document.createElement('div')
    root.setAttribute(REPLAY_ATTR, '')
    const a = document.createElement('button')
    const b = document.createElement('button')
    root.append(a, b)
    document.body.appendChild(root)

    clickAsUser(a, 'pointerdown')
    clickAsUser(a, 'mousedown')
    clickAsUser(b) // bare keyboard click on the sibling

    const onAPointerDown = vi.fn()
    const onBPointerDown = vi.fn()
    const onBClick = vi.fn()
    a.addEventListener('pointerdown', onAPointerDown)
    b.addEventListener('pointerdown', onBPointerDown)
    b.addEventListener('click', onBClick)

    expect(consumePendingReplay(root)).toBe(true)
    expect(onBClick).toHaveBeenCalledTimes(1)
    expect(onBPointerDown).not.toHaveBeenCalled()
    expect(onAPointerDown).not.toHaveBeenCalled()
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
    const { root } = mountControl()
    const loose = document.createElement('button')
    document.body.appendChild(loose)

    clickAsUser(loose)
    expect(consumePendingReplay(root)).toBe(false)
  })

  it('ignores untrusted clicks, so scripted input is never queued', () => {
    const { root, button } = mountControl()
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(store.buffer.get(root)).toBeUndefined()
    expect(consumePendingReplay(root)).toBe(false)
  })

  it('does not replay into a control disabled since the click', () => {
    // dispatchEvent ignores the disabled attribute — that only blocks
    // user-initiated activation — so without an explicit guard a control that
    // shipped enabled and hydrated disabled would still fire its handler.
    const { root, button } = mountControl()
    const onClick = vi.fn()
    clickAsUser(button)
    button.addEventListener('click', onClick)

    button.disabled = true

    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('does not replay into an input disabled since the click', () => {
    // The guard's selector has three arms; each needs its own test or the
    // "every guard has a failing mutation" claim is not true of all of them.
    const root = document.createElement('div')
    root.setAttribute(REPLAY_ATTR, '')
    const input = document.createElement('input')
    root.appendChild(input)
    document.body.appendChild(root)

    const onClick = vi.fn()
    clickAsUser(input)
    input.addEventListener('click', onClick)
    input.disabled = true

    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('does not replay into a control marked aria-disabled since the click', () => {
    const { root, button } = mountControl()
    const onClick = vi.fn()
    clickAsUser(button)
    button.addEventListener('click', onClick)

    button.setAttribute('aria-disabled', 'true')

    expect(consumePendingReplay(root)).toBe(false)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('is inert when the capture script never ran', () => {
    const { root } = mountControl()
    delete window.__phClickReplay

    expect(() => consumePendingReplay(root)).not.toThrow()
    expect(root.dataset.replayHydrated).toBe('1')
  })
})

describe('the shipped script string', () => {
  it('contains no sequence that would break out of the script tag', () => {
    // It is injected with dangerouslySetInnerHTML, so this invariant belongs
    // next to the string rather than asserted in a comment in the consumer.
    expect(CLICK_REPLAY_SCRIPT.toLowerCase()).not.toContain('</script')
  })
})

describe('script re-entry', () => {
  let teardownBridge: () => void
  beforeAll(() => { teardownBridge = installTrustedEventBridge() })
  afterAll(() => teardownBridge())

  it('a second evaluation is a no-op and keeps what was already captured', () => {
    // A browser re-executes an inline script if its node is re-inserted.
    // Without the guard, the second run would install a fresh empty buffer and
    // silently discard every click captured so far.
    const previous = window.__phClickReplay
    delete window.__phClickReplay
    new Function(CLICK_REPLAY_SCRIPT)()
    const store = window.__phClickReplay!

    const root = document.createElement('div')
    root.setAttribute(REPLAY_ATTR, '')
    const button = document.createElement('button')
    root.appendChild(button)
    document.body.appendChild(root)

    clickAsUser(button)
    expect(store.buffer.get(root)).toBeDefined()

    new Function(CLICK_REPLAY_SCRIPT)() // re-insertion
    expect(window.__phClickReplay).toBe(store)

    const onClick = vi.fn()
    button.addEventListener('click', onClick)
    expect(consumePendingReplay(root)).toBe(true)
    expect(onClick).toHaveBeenCalledTimes(1)

    store.stop()
    window.__phClickReplay = previous
  })
})

describe('capture teardown', () => {
  // Its own describe block: this installs a second, independently stoppable
  // copy of the script so stopping it cannot disturb the suite above.
  let teardownBridge: () => void

  beforeAll(() => {
    teardownBridge = installTrustedEventBridge()
  })
  afterAll(() => teardownBridge())

  it('stops buffering once capture is torn down', () => {
    const previous = window.__phClickReplay
    // Explicit: the script's re-entry guard makes a second evaluation a no-op
    // unless the global is cleared first. Without this the suite passed by
    // accident, aliasing the other describe's store and asserting nothing.
    delete window.__phClickReplay
    new Function(CLICK_REPLAY_SCRIPT)()
    const store = window.__phClickReplay!
    expect(store).not.toBe(previous)

    const root = document.createElement('div')
    root.setAttribute(REPLAY_ATTR, '')
    const button = document.createElement('button')
    root.appendChild(button)
    document.body.appendChild(root)

    clickAsUser(button)
    expect(store.buffer.get(root)).toBeDefined()

    store.stop()

    const after = document.createElement('div')
    after.setAttribute(REPLAY_ATTR, '')
    const afterButton = document.createElement('button')
    after.appendChild(afterButton)
    document.body.appendChild(after)

    clickAsUser(afterButton)
    expect(store.buffer.get(after)).toBeUndefined()
    expect(consumePendingReplay(after)).toBe(false)

    // Teardown must NOT discard what was already captured: a click taken just
    // before the cutoff, on a control that hydrates just after it, is younger
    // than the staleness limit and still deserves to replay.
    const onClick = vi.fn()
    button.addEventListener('click', onClick)
    expect(consumePendingReplay(root)).toBe(true)
    expect(onClick).toHaveBeenCalledTimes(1)

    window.__phClickReplay = previous
  })
})
