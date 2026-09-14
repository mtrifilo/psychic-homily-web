/**
 * Ownership-scoped timer tracking for "no timer survives unmount" assertions.
 *
 * `vi.getTimerCount()` counts every pending fake timer in the worker, which
 * includes timers scheduled by dependencies. Those are not part of any
 * component's unmount contract, and whether they are pending at a given moment
 * depends on real time elapsed between tests, so counting them makes an unmount
 * assertion sensitive to machine load.
 *
 * `trackAppTimers()` counts only the timers scheduled by first-party code. A
 * timer belongs to the frame that scheduled it: the first stack frame below this
 * module. A frame inside `node_modules` is library-owned and untracked; anything
 * else is app-owned and tracked until it is cleared or fires.
 *
 * Comparing against a baseline count instead only holds when the dependency's
 * timers are already pending when the baseline is taken. Ownership does not
 * depend on when a timer is armed, so it holds either way.
 *
 * Install it AFTER `vi.useFakeTimers()`, so the wrapped functions are the fake
 * ones, and call `restore()` before `vi.useRealTimers()`.
 */

const HELPER_FILE = new URL(import.meta.url).pathname.split('/').pop() ?? ''

/** The frame that called into this module, or undefined when there is none. */
function callerFrame(stack: string | undefined): string | undefined {
  return stack
    ?.split('\n')
    .slice(1)
    .map(line => line.trim())
    .find(line => line.startsWith('at ') && !line.includes(HELPER_FILE))
}

/**
 * True when the scheduling call site is first-party code. A call site that
 * cannot be attributed counts as app-owned, so broken attribution surfaces as a
 * failing assertion rather than a silently vacuous one.
 */
export function isAppOwnedCaller(caller: string | undefined): boolean {
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
  describe(): string
  /** Put the original timer functions back. */
  restore(): void
}

export function trackAppTimers(): AppTimerTracker {
  const realSetTimeout = globalThis.setTimeout
  const realClearTimeout = globalThis.clearTimeout
  const realSetInterval = globalThis.setInterval
  const realClearInterval = globalThis.clearInterval

  const pending = new Map<unknown, string>()

  /* eslint-disable @typescript-eslint/no-explicit-any */
  function schedule(real: any, args: any[], oneShot: boolean): any {
    const caller = callerFrame(new Error().stack)
    const [fn, ...rest] = args
    if (!isAppOwnedCaller(caller) || typeof fn !== 'function') {
      return real.call(globalThis, ...args)
    }

    // The id is only known once the timer is scheduled, so the callback reads it
    // through this handle. A one-shot timer stops being pending when it fires;
    // an interval only stops when it is cleared.
    const handle: { id?: unknown } = {}
    const tracked = oneShot
      ? (...callArgs: any[]) => {
          pending.delete(handle.id)
          return fn(...callArgs)
        }
      : fn
    handle.id = real.call(globalThis, tracked, ...rest)
    pending.set(handle.id, caller ?? '<no caller frame>')
    return handle.id
  }

  const wrappedSetTimeout = ((...args: any[]) =>
    schedule(realSetTimeout, args, true)) as typeof globalThis.setTimeout
  const wrappedSetInterval = ((...args: any[]) =>
    schedule(realSetInterval, args, false)) as typeof globalThis.setInterval
  const wrappedClearTimeout = ((id: any) => {
    pending.delete(id)
    return (realClearTimeout as any).call(globalThis, id)
  }) as typeof globalThis.clearTimeout
  const wrappedClearInterval = ((id: any) => {
    pending.delete(id)
    return (realClearInterval as any).call(globalThis, id)
  }) as typeof globalThis.clearInterval

  globalThis.setTimeout = wrappedSetTimeout
  globalThis.setInterval = wrappedSetInterval
  globalThis.clearTimeout = wrappedClearTimeout
  globalThis.clearInterval = wrappedClearInterval
  /* eslint-enable @typescript-eslint/no-explicit-any */

  return {
    pending: () => pending.size,
    describe: () =>
      `app-owned timers still pending:\n${[...pending.values()].join('\n')}`,
    // Each slot is restored only while it still holds this tracker's wrapper, so
    // a restore that runs after vi.useRealTimers() cannot reinstate a dead fake.
    restore: () => {
      if (globalThis.setTimeout === wrappedSetTimeout)
        globalThis.setTimeout = realSetTimeout
      if (globalThis.clearTimeout === wrappedClearTimeout)
        globalThis.clearTimeout = realClearTimeout
      if (globalThis.setInterval === wrappedSetInterval)
        globalThis.setInterval = realSetInterval
      if (globalThis.clearInterval === wrappedClearInterval)
        globalThis.clearInterval = realClearInterval
    },
  }
}
