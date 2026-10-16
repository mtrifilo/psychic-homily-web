import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readAnimationsAfterSettle } from './animations'

const READ_MS = 1_000

/** An animation whose `finished` the test settles, listed until it settles. */
class ScriptedAnimation {
  finished: Promise<unknown>
  settle!: (outcome: 'finish' | 'cancel') => void

  constructor(owner: FakeElement) {
    this.finished = new Promise((resolve, reject) => {
      this.settle = (outcome) => {
        owner.animations = owner.animations.filter((a) => a !== this)
        if (outcome === 'finish') resolve(this)
        else reject(new DOMException('The user aborted a request.', 'AbortError'))
      }
    })
    // A cancelled animation's rejection is absorbed by the read, not left
    // unhandled when the test settles it first.
    this.finished.catch(() => {})
  }
}

/** A height transition, named by its property. */
class ScriptedTransition extends ScriptedAnimation {
  transitionProperty = 'height'
}

class FakeElement {
  animations: ScriptedAnimation[] = []
  getAnimations() {
    return [...this.animations]
  }
  add<T extends ScriptedAnimation>(make: new (owner: FakeElement) => T): T {
    const animation = new make(this)
    this.animations.push(animation)
    return animation
  }
}

const read = (el: FakeElement) => readAnimationsAfterSettle(el as unknown as Element, READ_MS)

describe('readAnimationsAfterSettle', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('reads nothing running, with no timer left, for an element with no animations', async () => {
    await expect(read(new FakeElement())).resolves.toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('returns as the listed animations finish, before the read time, and clears its timer', async () => {
    const el = new FakeElement()
    const transition = el.add(ScriptedTransition)
    const other = el.add(ScriptedAnimation)
    const result = read(el)

    transition.settle('finish')
    other.settle('finish')

    await expect(result).resolves.toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('counts a cancelled or replaced animation as finished', async () => {
    const el = new FakeElement()
    const transition = el.add(ScriptedTransition)
    const result = read(el)

    transition.settle('cancel')

    await expect(result).resolves.toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('names what still runs once the read time passes: a transition by property, others by class', async () => {
    const el = new FakeElement()
    el.add(ScriptedTransition)
    el.add(ScriptedAnimation)
    const result = read(el)

    await vi.advanceTimersByTimeAsync(READ_MS - 1)
    let done = false
    void result.then(() => (done = true))
    await Promise.resolve()
    expect(done).toBe(false)

    await vi.advanceTimersByTimeAsync(1)
    await expect(result).resolves.toEqual(['height', 'ScriptedAnimation'])
  })
})
