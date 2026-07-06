import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { useLazyGraphMount } from './useLazyGraphMount'

function Probe({ rootMargin }: { rootMargin?: string }) {
  const { containerRef, isMounted } = useLazyGraphMount(rootMargin)
  return (
    <div ref={containerRef} data-testid="probe">
      {isMounted ? 'mounted' : 'pending'}
    </div>
  )
}

// Controllable IntersectionObserver: capture the callback + options so the test
// can drive intersection and assert the rootMargin. test/setup.ts installs a
// never-intersecting stub globally; vi.stubGlobal overrides it per test.
let ioCallback: IntersectionObserverCallback | null = null
let ioOptions: IntersectionObserverInit | undefined
let observedCount = 0
let disconnected = false

class MockIO {
  constructor(cb: IntersectionObserverCallback, options?: IntersectionObserverInit) {
    ioCallback = cb
    ioOptions = options
  }
  observe() {
    observedCount += 1
  }
  disconnect() {
    disconnected = true
  }
  unobserve() {}
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
}

function fire(isIntersecting: boolean) {
  act(() => {
    ioCallback?.(
      [{ isIntersecting } as IntersectionObserverEntry],
      {} as IntersectionObserver,
    )
  })
}

// test/setup.ts installs a NON-configurable IntersectionObserver mock, so
// vi.stubGlobal can't redefine it — replace via plain (writable) assignment,
// the same pattern HomeSceneGraph.test uses, and restore after each test.
const OriginalIO = window.IntersectionObserver
function setIO(value: unknown) {
  window.IntersectionObserver = value as typeof IntersectionObserver
}

describe('useLazyGraphMount', () => {
  beforeEach(() => {
    ioCallback = null
    ioOptions = undefined
    observedCount = 0
    disconnected = false
  })

  afterEach(() => {
    window.IntersectionObserver = OriginalIO
  })

  it('starts pending and mounts once the observed element intersects, then disconnects', () => {
    setIO(MockIO)
    render(<Probe />)
    expect(screen.getByTestId('probe')).toHaveTextContent('pending')
    expect(observedCount).toBe(1)

    fire(true)

    expect(screen.getByTestId('probe')).toHaveTextContent('mounted')
    expect(disconnected).toBe(true)
  })

  it('stays pending for a non-intersecting entry', () => {
    setIO(MockIO)
    render(<Probe />)
    fire(false)
    expect(screen.getByTestId('probe')).toHaveTextContent('pending')
  })

  it('passes the rootMargin to the observer (default 200px, overridable)', () => {
    setIO(MockIO)
    const { unmount } = render(<Probe />)
    expect(ioOptions?.rootMargin).toBe('200px')
    unmount()

    render(<Probe rootMargin="0px" />)
    expect(ioOptions?.rootMargin).toBe('0px')
  })

  it('falls back to an immediate (microtask-deferred) mount when IntersectionObserver is unavailable', async () => {
    setIO(undefined)
    render(<Probe />)
    // The fallback defers its setState to a microtask (react-hooks/
    // set-state-in-effect), so it is NOT mounted synchronously.
    expect(screen.getByTestId('probe')).toHaveTextContent('pending')
    await act(async () => {
      await Promise.resolve()
    })
    expect(screen.getByTestId('probe')).toHaveTextContent('mounted')
  })
})
