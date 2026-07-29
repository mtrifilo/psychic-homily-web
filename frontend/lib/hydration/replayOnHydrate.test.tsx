import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest'
import { act } from '@testing-library/react'
import { hydrateRoot } from 'react-dom/client'
import { renderToString } from 'react-dom/server'
import { CLICK_REPLAY_SCRIPT, replayOnHydrate } from './clickReplay'
import { BracketLink } from '@/components/shared/BracketLink'
import { installTrustedEventBridge, clickAsUser } from './testing/trustedEvents'

/**
 * These tests drive the real sequence: server HTML, a click against it while
 * React is absent, then an actual `hydrateRoot`. Nothing short of that
 * exercises the ref-callback timing the primitive depends on.
 */
function Toggle({ onActivate }: { onActivate: () => void }) {
  return (
    <button {...replayOnHydrate} onClick={onActivate}>
      Toggle
    </button>
  )
}

describe('replayOnHydrate', () => {
  let store: NonNullable<Window['__phClickReplay']>
  let teardownBridge: () => void

  beforeAll(() => {
    teardownBridge = installTrustedEventBridge()
    new Function(CLICK_REPLAY_SCRIPT)()
    store = window.__phClickReplay!
  })

  afterAll(() => teardownBridge())

  beforeEach(() => {
    document.body.innerHTML = ''
    window.__phClickReplay = store
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
    expect(store.buffer.get(button)).toBeUndefined()
  })

  it('replays through BracketLink, which owns replay for every bracket control', () => {
    // End-to-end through the shared primitive, not through a local <button>.
    // This is what catches a BracketLink refactor that keeps the marker
    // attribute but loses the composed replay ref — markup-only assertions
    // cannot tell those apart, and ~71 controls depend on this path.
    const onActivate = vi.fn()
    const node = <BracketLink label="Save" onClick={onActivate} />

    const container = document.createElement('div')
    container.innerHTML = renderToString(node)
    document.body.appendChild(container)
    const button = container.querySelector('button')!

    clickAsUser(button, 'pointerdown')
    clickAsUser(button)
    expect(onActivate).not.toHaveBeenCalled()

    act(() => {
      hydrateRoot(container, node)
    })

    expect(onActivate).toHaveBeenCalledTimes(1)
  })

  it('renders the marker attribute into the server HTML', () => {
    // If the attribute stopped reaching server HTML, nothing would ever be
    // buffered and every other test here would still pass.
    expect(renderToString(<Toggle onActivate={() => {}} />)).toContain(
      'data-replay-on-hydrate'
    )
  })
})
