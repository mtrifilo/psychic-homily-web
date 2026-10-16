import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readAnimationsAfterSettle } from './animations'

const READ_MS = 1_000

/** Stands in for the browser's CSSTransition, which jsdom does not define. */
class FakeTransition {
  transitionProperty = 'height'
}

/** Any other animation, named by its class. */
class FakeAnimation {}

/**
 * An element whose listed animations the test settles: settling one removes
 * it from the list and resolves (or, cancelled, rejects) its `finished`.
 */
function fakeElement() {
  let listed: { finished: Promise<unknown> }[] = []
  const add = (Kind: typeof FakeTransition | typeof FakeAnimation) => {
    let settle!: (outcome: 'finish' | 'cancel') => void
    const animation = Object.assign(new Kind(), {
      finished: new Promise((resolve, reject) => {
        settle = (outcome) => {
          listed = listed.filter((a) => a !== animation)
          if (outcome === 'finish') resolve(animation)
          else reject(new DOMException('The user aborted a request.', 'AbortError'))
        }
      }),
    })
    // A rejection the read has not subscribed to yet is not left unhandled.
    animation.finished.catch(() => {})
    listed.push(animation)
    return settle
  }
  const el = { getAnimations: () => [...listed] } as unknown as Element
  return { el, add }
}

describe('readAnimationsAfterSettle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('CSSTransition', FakeTransition)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('reads nothing running, with no timer left, for an element with no animations', async () => {
    const { el } = fakeElement()
    await expect(readAnimationsAfterSettle(el, READ_MS)).resolves.toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('returns as the listed animations finish, before the read time, and clears its timer', async () => {
    const { el, add } = fakeElement()
    const settleTransition = add(FakeTransition)
    const settleOther = add(FakeAnimation)
    const read = readAnimationsAfterSettle(el, READ_MS)

    settleTransition('finish')
    settleOther('finish')

    await expect(read).resolves.toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('counts a cancelled or replaced animation as finished', async () => {
    const { el, add } = fakeElement()
    const settle = add(FakeTransition)
    const read = readAnimationsAfterSettle(el, READ_MS)

    settle('cancel')

    await expect(read).resolves.toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('names what still runs once the read time passes: a transition by property, others by class', async () => {
    const { el, add } = fakeElement()
    add(FakeTransition)
    add(FakeAnimation)
    const read = readAnimationsAfterSettle(el, READ_MS)

    await vi.advanceTimersByTimeAsync(READ_MS)

    await expect(read).resolves.toEqual(['height', 'FakeAnimation'])
  })
})
