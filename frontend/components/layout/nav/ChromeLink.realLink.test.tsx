import { describe, it, expect, vi, beforeEach, afterEach, afterAll } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { AppRouterContext } from 'next/dist/shared/lib/app-router-context.shared-runtime'
import { atlasItem } from './navData'

const pathname = vi.hoisted(() => ({ current: '' }))
vi.mock('next/navigation', async importOriginal => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  usePathname: () => pathname.current,
}))

// Next's compiler aliases `next/link` to the App Router Link in app code, and
// `react-server-dom-webpack/client` to its compiled copy
// (next/dist/build/create-compiler-aliases.js). Vitest applies neither: it
// would resolve the Pages Router Link, which observes every anchor whatever
// `prefetch` says. Both aliases are mirrored here, for this file's Link load
// only.
vi.mock('next/link', async () => {
  const { default: Module } = await import('node:module')
  const resolver = Module as unknown as {
    _resolveFilename: (request: string, ...rest: unknown[]) => string
  }
  const resolve = resolver._resolveFilename
  resolver._resolveFilename = function (request: string, ...rest: unknown[]) {
    const aliased =
      request === 'react-server-dom-webpack/client'
        ? 'next/dist/compiled/react-server-dom-webpack/client'
        : request
    return resolve.call(this, aliased, ...rest)
  }
  // The compiled Flight client reads webpack's module hooks at load; nothing
  // here resolves a server reference, so inert stubs are enough.
  const hooks = globalThis as unknown as Record<string, unknown>
  hooks.__webpack_require__ ??= () => ({})
  hooks.__webpack_chunk_load__ ??= () => Promise.resolve()
  hooks.__chromeLinkTestWebpackStubs = true
  // The aliased require runs while the Link's module graph loads, so the
  // resolver is restored as soon as that load returns.
  try {
    const link = await vi.importActual<{ default?: unknown }>('next/dist/client/app-dir/link')
    return { default: link.default ?? link }
  } finally {
    resolver._resolveFilename = resolve
  }
})

/**
 * The hold relies on Next's own Link: with `prefetch={false}` it never hands
 * the anchor to its shared IntersectionObserver, and when the prop flips back
 * it does, because the Link's ref callback depends on whether prefetch is
 * enabled. This file runs the REAL next/link and watches that observer (the
 * setup's IntersectionObserver mock, whose prototype Next's instance shares),
 * so a Next upgrade that stops re-observing on the flip fails here.
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

afterAll(() => {
  const hooks = globalThis as unknown as Record<string, unknown>
  if (hooks.__chromeLinkTestWebpackStubs) {
    delete hooks.__webpack_require__
    delete hooks.__webpack_chunk_load__
    delete hooks.__chromeLinkTestWebpackStubs
  }
})

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
        <ChromeLink href="/shows">Shows</ChromeLink>
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
