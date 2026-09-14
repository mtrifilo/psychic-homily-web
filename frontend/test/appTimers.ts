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
 * True when the scheduling call site is first-party code. A stack with no frame
 * below this module counts as app-owned, so broken attribution surfaces as a
 * failing assertion rather than a silently vacuous one.
 */
export function isAppOwnedStack(stack: string | undefined): boolean {
  const caller = callerFrame(stack)
  return caller === undefined || !caller.includes('/node_modules/')
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
    const stack = new Error().stack
    if (!isAppOwnedStack(stack)) return real(...args)

    const [fn, ...rest] = args
    // The id is only known once the timer is scheduled, so the callback reads it
    // through this handle. A one-shot timer stops being pending when it fires;
    // an interval only stops when it is cleared.
    const handle: { id?: unknown } = {}
    const tracked =
      oneShot && typeof fn === 'function'
        ? (...callArgs: any[]) => {
            pending.delete(handle.id)
            return fn(...callArgs)
          }
        : fn
    handle.id = real(tracked, ...rest)
    pending.set(handle.id, callerFrame(stack) ?? '<no caller frame>')
    return handle.id
  }

  globalThis.setTimeout = ((...args: any[]) =>
    schedule(realSetTimeout, args, true)) as any
  globalThis.setInterval = ((...args: any[]) =>
    schedule(realSetInterval, args, false)) as any
  globalThis.clearTimeout = ((id: any) => {
    pending.delete(id)
    return (realClearTimeout as any)(id)
  }) as any
  globalThis.clearInterval = ((id: any) => {
    pending.delete(id)
    return (realClearInterval as any)(id)
  }) as any
  /* eslint-enable @typescript-eslint/no-explicit-any */

  return {
    pending: () => pending.size,
    describe: () =>
      pending.size === 0
        ? 'no app-owned timers pending'
        : `app-owned timers still pending:\n${[...pending.values()].join('\n')}`,
    restore: () => {
      globalThis.setTimeout = realSetTimeout
      globalThis.clearTimeout = realClearTimeout
      globalThis.setInterval = realSetInterval
      globalThis.clearInterval = realClearInterval
    },
  }
}
