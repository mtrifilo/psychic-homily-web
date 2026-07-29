import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest'
import { act } from '@testing-library/react'
import { hydrateRoot } from 'react-dom/client'
import { renderToString } from 'react-dom/server'
import { useReplayOnHydrate } from './useReplayOnHydrate'
import { CLICK_REPLAY_SCRIPT, replayOnHydrate } from '@/lib/hydration/clickReplay'

/**
 * These tests drive the real thing: server HTML, a click against it while React
 * is absent, then an actual `hydrateRoot`. That is the sequence the primitive
 * exists for, and nothing short of it exercises the effect timing.
 *
 * See `lib/hydration/clickReplay.test.ts` for why trusted input has to be
 * forged through jsdom's impl object mid-dispatch.
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
  if (!impl) throw new Error('jsdom event impl not found')
  impl.isTrusted = true
}

const USER_EVENT_TYPES = ['pointerdown', 'click'] as const

function clickAsUser(target: Element, type = 'click') {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    button: 0,
  })
  intendedTrusted.add(event)
  target.dispatchEvent(event)
}

function Toggle({ onActivate }: { onActivate: () => void }) {
  const replayRef = useReplayOnHydrate<HTMLButtonElement>()
  return (
    <button ref={replayRef} {...replayOnHydrate} onClick={onActivate}>
      Toggle
    </button>
  )
}

describe('useReplayOnHydrate', () => {
  let store: NonNullable<Window['__phClickReplay']>

  beforeAll(() => {
    for (const type of USER_EVENT_TYPES) {
      window.addEventListener(type, reTrustDuringDispatch, true)
    }
    new Function(CLICK_REPLAY_SCRIPT)()
    store = window.__phClickReplay!
  })

  afterAll(() => {
    for (const type of USER_EVENT_TYPES) {
      window.removeEventListener(type, reTrustDuringDispatch, true)
    }
  })

  beforeEach(() => {
    document.body.innerHTML = ''
    store.buffer.clear()
  })

  /** Paint the server HTML, exactly as the browser receives it. */
  function paintServerHtml(onActivate: () => void) {
    const container = document.createElement('div')
    container.innerHTML = renderToString(<Toggle onActivate={onActivate} />)
    document.body.appendChild(container)
    return { container, button: container.querySelector('button')! }
  }

  it('replays a click that landed on the server HTML before hydration', () => {
    const onActivate = vi.fn()
    const { container, button } = paintServerHtml(onActivate)

    // The control is painted and clickable, but React is not here yet.
    clickAsUser(button, 'pointerdown')
    clickAsUser(button)
    expect(onActivate).not.toHaveBeenCalled()

    act(() => {
      hydrateRoot(container, <Toggle onActivate={onActivate} />)
    })

    expect(onActivate).toHaveBeenCalledTimes(1)
  })

  it('leaves a post-hydration click alone — no second fire', () => {
    const onActivate = vi.fn()
    const { container, button } = paintServerHtml(onActivate)

    act(() => {
      hydrateRoot(container, <Toggle onActivate={onActivate} />)
    })
    expect(onActivate).not.toHaveBeenCalled()

    act(() => {
      clickAsUser(button)
    })
    expect(onActivate).toHaveBeenCalledTimes(1)
  })

  it('marks the node hydrated so later clicks are never buffered', () => {
    const onActivate = vi.fn()
    const { container, button } = paintServerHtml(onActivate)

    act(() => {
      hydrateRoot(container, <Toggle onActivate={onActivate} />)
    })

    expect(button.dataset.replayHydrated).toBe('1')
    act(() => {
      clickAsUser(button)
    })
    expect(store.buffer.size).toBe(0)
  })
})
