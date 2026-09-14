import { onTestFinished } from 'vitest'

/**
 * Ownership-scoped timer tracking for "no timer survives unmount" assertions.
 *
 * `vi.getTimerCount()` counts every pending fake timer in the worker, including
 * ones a dependency schedules while the test drives the component. TanStack Form,
 * for example, throttles a form-state broadcast through `@tanstack/pacer-lite` on
 * field changes, and unmounting a form does not clear that throttle. Those timers
 * belong to no component's unmount contract.
 *
 * `trackAppTimers()` counts only the timers scheduled by first-party code. A timer
 * belongs to the frame that scheduled it: the first stack frame below this module.
 * A frame inside `node_modules` is library-owned and untracked; anything else is
 * app-owned and tracked until it is cleared or fires. A call site that cannot be
 * attributed counts as app-owned, so broken attribution fails an assertion instead
 * of quietly passing one.
 *
 * Every scheduler vitest's fake clock installs is tracked: `setTimeout`,
 * `setInterval`, `setImmediate`, `requestAnimationFrame` and `requestIdleCallback`,
 * each with its matching cancel. Microtasks (`queueMicrotask`, `process.nextTick`)
 * are not faked by vitest and are not tracked.
 *
 * Install it inside a test, after `vi.useFakeTimers()`, so the wrapped functions
 * are the fake ones. Cleanup is registered with `onTestFinished`; `restore()` is
 * available for earlier cleanup and is order-independent, since it reverts a slot
 * only while that slot still holds this tracker's wrapper. Only one tracker may be
 * installed at a time.
 *
 * A strict `vi.getTimerCount()` assertion stays correct for a tree that arms no
 * dependency timers. Reach for this when the tree does.
 */

const HELPER_FILE = `/${new URL(import.meta.url).pathname.split('/').pop()}`

const INSTALLED = Symbol.for('psychic-homily.appTimers.installed')

/** Scheduler/cancel pairs vitest's fake clock installs, with their lifetimes. */
const SCHEDULERS = [
  { set: 'setTimeout', clear: 'clearTimeout', oneShot: true },
  { set: 'setInterval', clear: 'clearInterval', oneShot: false },
  { set: 'setImmediate', clear: 'clearImmediate', oneShot: true },
  {
    set: 'requestAnimationFrame',
    clear: 'cancelAnimationFrame',
    oneShot: true,
  },
  { set: 'requestIdleCallback', clear: 'cancelIdleCallback', oneShot: true },
] as const

/** The frame that called into this module, or undefined when there is none. */
function callerFrame(stack: string | undefined): string | undefined {
  return stack
    ?.split('\n')
    .slice(1)
    .map(line => line.trim())
    .find(line => line.startsWith('at ') && !line.includes(HELPER_FILE))
}

function isAppOwnedCaller(caller: string | undefined): boolean {
  return caller === undefined || !caller.includes('/node_modules/')
}

/** Ownership of the call site recorded in `stack`. */
export function isAppOwnedStack(stack: string | undefined): boolean {
  return isAppOwnedCaller(callerFrame(stack))
}

export interface AppTimerTracker {
  /** Count of app-scheduled timers that are still pending. */
  pending(): number
  /** Call sites of the pending app-scheduled timers, for assertion messages. */
  pendingCallSites(): string
  /** Put the original scheduler functions back. */
  restore(): void
}

/* eslint-disable @typescript-eslint/no-explicit-any */
type AnyFn = (...args: any[]) => any

/** Copies the symbol keys a scheduler carries, such as `util.promisify.custom`. */
function adoptSymbols(wrapper: AnyFn, original: AnyFn): AnyFn {
  for (const key of Object.getOwnPropertySymbols(original)) {
    ;(wrapper as any)[key] = (original as any)[key]
  }
  return wrapper
}

export function trackAppTimers(): AppTimerTracker {
  const target = globalThis as any
  if (target[INSTALLED]) {
    throw new Error(
      'trackAppTimers is already installed; restore the existing tracker first'
    )
  }

  const pending = new Map<string, string>()
  const restores: Array<() => void> = []

  for (const { set, clear, oneShot } of SCHEDULERS) {
    const realSet: AnyFn | undefined = target[set]
    const realClear: AnyFn | undefined = target[clear]
    if (typeof realSet !== 'function' || typeof realClear !== 'function') continue

    const key = (id: unknown) => `${set}:${String(id)}`

    const wrappedSet = adoptSymbols((...args: any[]) => {
      const caller = callerFrame(new Error().stack)
      const [fn, ...rest] = args
      if (!isAppOwnedCaller(caller) || typeof fn !== 'function') {
        return realSet.call(globalThis, ...args)
      }
      // The id is only known once the timer is scheduled, so a one-shot callback
      // reads it back through this handle to stop counting itself when it fires.
      const handle: { id?: unknown } = {}
      const tracked = oneShot
        ? (...callArgs: any[]) => {
            pending.delete(key(handle.id))
            return fn(...callArgs)
          }
        : fn
      handle.id = realSet.call(globalThis, tracked, ...rest)
      pending.set(key(handle.id), caller ?? '<no caller frame>')
      return handle.id
    }, realSet)

    const wrappedClear = adoptSymbols((id: any) => {
      pending.delete(key(id))
      return realClear.call(globalThis, id)
    }, realClear)

    target[set] = wrappedSet
    target[clear] = wrappedClear
    restores.push(() => {
      if (target[set] === wrappedSet) target[set] = realSet
      if (target[clear] === wrappedClear) target[clear] = realClear
    })
  }

  target[INSTALLED] = true

  const restore = () => {
    for (const undo of restores) undo()
    delete target[INSTALLED]
  }
  onTestFinished(restore)

  return {
    pending: () => pending.size,
    pendingCallSites: () =>
      `app-owned timers still pending:\n${[...pending.values()].join('\n')}`,
    restore,
  }
}
/* eslint-enable @typescript-eslint/no-explicit-any */
