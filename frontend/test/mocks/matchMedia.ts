import { act } from '@testing-library/react'

/**
 * Controllable `window.matchMedia` test shim: each query answers from a
 * table a test can flip mid-render, and a flip notifies that query's
 * `change` listeners inside `act`, the way a real viewport crossing does.
 * Queries missing from the table answer false.
 *
 * Usage:
 *   const mm = installMatchMedia({ [QUERY]: true })   // before render
 *   mm.set(QUERY, false)                              // after render
 *   mm.restore()                                      // in afterEach
 */
interface MatchMediaControls {
  /** Change a query's answer and notify its listeners. */
  set: (query: string, matches: boolean) => void
  /** Every query string asked for since install, in order. */
  queries: string[]
  /** Restore the matchMedia that was installed before. */
  restore: () => void
}

export function installMatchMedia(
  initial: Record<string, boolean> = {},
): MatchMediaControls {
  const original = window.matchMedia
  const answers = { ...initial }
  const listeners = new Map<string, Set<() => void>>()
  const queries: string[] = []

  window.matchMedia = ((query: string) => {
    queries.push(query)
    const forQuery = () => {
      if (!listeners.has(query)) listeners.set(query, new Set())
      return listeners.get(query)!
    }
    return {
      get matches() {
        return answers[query] ?? false
      },
      media: query,
      onchange: null,
      addEventListener: (_: string, fn: () => void) => forQuery().add(fn),
      removeEventListener: (_: string, fn: () => void) => forQuery().delete(fn),
      addListener: (fn: () => void) => forQuery().add(fn),
      removeListener: (fn: () => void) => forQuery().delete(fn),
      dispatchEvent: () => false,
    }
  }) as unknown as typeof window.matchMedia

  return {
    set(query, matches) {
      answers[query] = matches
      act(() => listeners.get(query)?.forEach((fn) => fn()))
    },
    queries,
    restore() {
      window.matchMedia = original
    },
  }
}
