import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useActiveSection } from './useActiveSection'

const ANCHORS = ['first', 'second', 'third'] as const

/** Section tops in viewport coordinates, as the scroll position left them. */
let tops: Record<string, number>

/** The hook's root; a decoy outside it carries a tracked id. */
let root: HTMLDivElement
const rootRef = { current: null as HTMLElement | null }

function mountSections() {
  document.body.innerHTML = ''
  // Another route's tree left mounted and hidden, carrying the same ids,
  // ahead of the hub in document order. Its box reads as passed.
  const decoy = document.createElement('div')
  decoy.hidden = true
  for (const anchor of ['third', 'second-row']) {
    const stale = document.createElement('div')
    stale.id = anchor
    stale.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
    decoy.appendChild(stale)
  }
  document.body.appendChild(decoy)
  root = document.createElement('div')
  document.body.appendChild(root)
  rootRef.current = root
  for (const anchor of ANCHORS) {
    const section = document.createElement('section')
    section.id = anchor
    // What the hub's `scroll-mt-[calc(var(--topbar-height)+1rem)]` resolves to.
    section.style.scrollMarginTop = '72px'
    section.getBoundingClientRect = () =>
      ({ top: tops[anchor] }) as DOMRect
    if (anchor === 'second') {
      const row = document.createElement('div')
      row.id = 'second-row'
      section.appendChild(row)
    }
    root.appendChild(section)
  }
}

function setScroll({
  scrollY,
  innerHeight,
  scrollHeight,
}: {
  scrollY: number
  innerHeight: number
  scrollHeight: number
}) {
  Object.defineProperty(window, 'scrollY', { configurable: true, value: scrollY })
  Object.defineProperty(window, 'innerHeight', {
    configurable: true,
    value: innerHeight,
  })
  Object.defineProperty(document.documentElement, 'scrollHeight', {
    configurable: true,
    value: scrollHeight,
  })
}

function scroll() {
  act(() => {
    window.dispatchEvent(new Event('scroll'))
    vi.runOnlyPendingTimers()
  })
}

describe('useActiveSection', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'requestAnimationFrame', 'cancelAnimationFrame'] })
    window.history.replaceState(null, '', '/settings')
    tops = { first: 80, second: 900, third: 1800 }
    setScroll({ scrollY: 0, innerHeight: 800, scrollHeight: 3000 })
    mountSections()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('starts on the first section', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    expect(result.current.activeAnchor).toBe('first')
  })

  it('marks the last section whose top has passed the line under the TopBar', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))

    // The reading line is the 72px scroll margin: a top at 72 has passed it.
    tops = { first: -700, second: 72, third: 900 }
    setScroll({ scrollY: 800, innerHeight: 800, scrollHeight: 3000 })
    scroll()
    expect(result.current.activeAnchor).toBe('second')

    tops = { first: -700, second: 73, third: 900 }
    scroll()
    expect(result.current.activeAnchor).toBe('first')
  })

  it('reads only its own root, not a hidden tree carrying the same ids', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))

    tops = { first: 40, second: 900, third: 1800 }
    setScroll({ scrollY: 10, innerHeight: 800, scrollHeight: 3000 })
    scroll()
    expect(result.current.activeAnchor).toBe('first')
  })

  it('marks the last section at the bottom of a scrolled page', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))

    tops = { first: -2000, second: -1100, third: 400 }
    setScroll({ scrollY: 2200, innerHeight: 800, scrollHeight: 3000 })
    scroll()
    expect(result.current.activeAnchor).toBe('third')
  })

  it('marks a cold-loaded fragment outright and holds it while the jump settles', () => {
    window.history.replaceState(null, '', '/settings#third')
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    expect(result.current.activeAnchor).toBe('third')

    // The jump cannot bring a short last-but-one section to the line; the
    // scroll it causes must not override the fragment.
    tops = { first: -1000, second: 100, third: 500 }
    act(() => {
      window.dispatchEvent(new Event('scroll'))
      vi.advanceTimersByTime(16)
    })
    expect(result.current.activeAnchor).toBe('third')

    // Once settled, reading position rules again.
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    scroll()
    expect(result.current.activeAnchor).toBe('first')
  })

  it('measures again when the hold ends on its own', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    act(() => result.current.select('third'))

    // A history traversal restored a scroll position during the hold.
    tops = { first: -700, second: 40, third: 900 }
    setScroll({ scrollY: 800, innerHeight: 800, scrollHeight: 3000 })
    act(() => {
      window.dispatchEvent(new Event('scroll'))
      vi.advanceTimersByTime(16)
    })
    expect(result.current.activeAnchor).toBe('third')

    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.activeAnchor).toBe('second')
  })

  it('ends the hold as soon as the viewer scrolls on their own', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    act(() => result.current.select('third'))

    tops = { first: -700, second: 40, third: 900 }
    setScroll({ scrollY: 800, innerHeight: 800, scrollHeight: 3000 })
    act(() => {
      window.dispatchEvent(new Event('wheel'))
    })
    expect(result.current.activeAnchor).toBe('second')
  })

  it('keeps a chosen section marked at the bottom until the viewer scrolls', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    act(() => result.current.select('second'))

    // The page bottomed out before the chosen section reached the line.
    tops = { first: -1500, second: 300, third: 600 }
    setScroll({ scrollY: 2200, innerHeight: 800, scrollHeight: 3000 })
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.activeAnchor).toBe('second')
    scroll()
    expect(result.current.activeAnchor).toBe('second')

    // Once the viewer scrolls on their own, the bottom marks the last section.
    act(() => {
      window.dispatchEvent(new Event('wheel'))
    })
    scroll()
    expect(result.current.activeAnchor).toBe('third')
  })

  it('forgets a chosen section once its top leaves the screen', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    act(() => result.current.select('second'))
    act(() => {
      vi.advanceTimersByTime(1000)
    })

    // A scroll with no wheel, touch or key input (a dragged scrollbar) carries
    // the chosen section off screen, then back down to the bottom.
    tops = { first: 80, second: 900, third: 1800 }
    setScroll({ scrollY: 0, innerHeight: 800, scrollHeight: 3000 })
    scroll()
    expect(result.current.activeAnchor).toBe('first')

    tops = { first: -1500, second: 300, third: 600 }
    setScroll({ scrollY: 2200, innerHeight: 800, scrollHeight: 3000 })
    scroll()
    expect(result.current.activeAnchor).toBe('third')
  })

  it('maps a fragment for a row inside a section to that section', () => {
    window.history.replaceState(null, '', '/settings#second-row')
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    expect(result.current.activeAnchor).toBe('second')
  })

  it('follows a hash change and ignores fragments it does not track', () => {
    const { result } = renderHook(() => useActiveSection(ANCHORS, rootRef))

    act(() => {
      window.history.replaceState(null, '', '/settings#second')
      window.dispatchEvent(new HashChangeEvent('hashchange'))
    })
    expect(result.current.activeAnchor).toBe('second')

    act(() => {
      window.history.replaceState(null, '', '/settings#elsewhere')
      window.dispatchEvent(new HashChangeEvent('hashchange'))
    })
    expect(result.current.activeAnchor).toBe('second')
  })

  it('selects on demand and leaves no timer behind on unmount', () => {
    const { result, unmount } = renderHook(() => useActiveSection(ANCHORS, rootRef))
    act(() => result.current.select('third'))
    expect(result.current.activeAnchor).toBe('third')
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})
