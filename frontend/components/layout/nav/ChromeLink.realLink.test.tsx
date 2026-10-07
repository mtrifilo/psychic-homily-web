import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { AppRouterContext } from 'next/dist/shared/lib/app-router-context.shared-runtime'
import { atlasItem } from './navData'

const pathname = vi.hoisted(() => ({ current: '' }))
vi.mock('next/navigation', async importOriginal => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  usePathname: () => pathname.current,
}))

/**
 * The hold relies on Next's own Link: with `prefetch={false}` it never hands
 * the anchor to its shared IntersectionObserver, and when the prop flips back
 * it does, because the Link's ref callback depends on whether prefetch is
 * enabled. This file runs the REAL next/link (the App Router build, which
 * vitest.config.mts resolves it to) and watches that observer (the setup's
 * IntersectionObserver mock, whose prototype Next's instance shares), so a
 * Next upgrade that stops re-observing on the flip fails here.
 */
const router = {
  back: vi.fn(),
  forward: vi.fn(),
  refresh: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  prefetch: vi.fn(),
  hmrRefresh: vi.fn(),
}

async function load() {
  vi.resetModules()
  const signal = await import('@/lib/atlasMapReady')
  const { ChromeLink } = await import('./ChromeLink')
  return { signal, ChromeLink }
}

describe('ChromeLink with the real next/link', () => {
  let observe: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    observe = vi.spyOn(window.IntersectionObserver.prototype, 'observe')
  })
  afterEach(() => {
    observe.mockRestore()
  })

  function observed(anchor: HTMLElement) {
    return observe.mock.calls.some(([element]: [Element]) => element === anchor)
  }

  it('observes the anchor for prefetch only once the Atlas hold releases', async () => {
    pathname.current = atlasItem.href
    const { signal, ChromeLink } = await load()
    render(
      <AppRouterContext.Provider value={router}>
        <ChromeLink href="/shows" atlasPrefetch="after-map">
          Shows
        </ChromeLink>
      </AppRouterContext.Provider>
    )
    const anchor = screen.getByText('Shows')
    expect(observed(anchor)).toBe(false)

    act(() => signal.markAtlasMapReady())

    expect(observed(anchor)).toBe(true)
  })

  it('observes the anchor at once on /shows', async () => {
    pathname.current = '/shows'
    const { ChromeLink } = await load()
    render(
      <AppRouterContext.Provider value={router}>
        <ChromeLink href="/venues">Venues</ChromeLink>
      </AppRouterContext.Provider>
    )
    expect(observed(screen.getByText('Venues'))).toBe(true)
  })
})
